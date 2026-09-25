package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/khansbikezone/bikezone-api/internal/domain"
	"github.com/khansbikezone/bikezone-api/internal/platform"
	"github.com/khansbikezone/bikezone-api/internal/store"
	"github.com/khansbikezone/bikezone-api/internal/store/gen"
)

// Products administers products as whole documents: the product, its
// attribute values, and its variants with their own values and prices are
// saved together in one transaction. Saving the whole document keeps the EAV
// rows and the JSONB filter projections consistent by construction, and it
// matches how both admin clients edit: one form, one save.
type Products struct {
	store *store.Store
}

// NewProducts builds the product administration service.
func NewProducts(st *store.Store) *Products { return &Products{store: st} }

// Document limits.
const (
	maxProductNameLen = 200
	maxSummaryLen     = 500
	maxProductDescLen = 20000
	maxSKULen         = 64
	maxCodeLen        = 100
	maxSuffixLen      = 200
	maxVariants       = 200
	maxAltTextLen     = 300
)

// ProductInput is a whole product document. Attribute values use the JSON
// shapes documented in schema.go, keyed by attribute key; product-level
// attributes go on the product, variant-axis attributes on each variant.
type ProductInput struct {
	CategoryID          uuid.UUID                  `json:"categoryId"`
	BrandID             *uuid.UUID                 `json:"brandId"`
	Name                string                     `json:"name"`
	Slug                string                     `json:"slug"` // optional: derived from the name on create, kept on update
	Summary             *string                    `json:"summary"`
	Description         *string                    `json:"description"`
	Status              domain.ProductStatus       `json:"status"` // default draft
	IsFeatured          bool                       `json:"isFeatured"`
	RetailPriceIsPublic bool                       `json:"retailPriceIsPublic"`
	Attributes          map[string]json.RawMessage `json:"attributes"`
	Variants            []VariantInput             `json:"variants"`
}

// VariantInput is one variant in a product document. ID identifies an
// existing variant to update; variants without one are created, and existing
// variants missing from the document are deleted. Prices are decimal strings
// keyed by tier; a tier that is omitted keeps its current price, and every
// change is recorded as a new price row effective today (price history).
type VariantInput struct {
	ID             *uuid.UUID                  `json:"id"`
	SKU            string                      `json:"sku"`
	SupplierID     *uuid.UUID                  `json:"supplierId"`
	SupplierItemNo *string                     `json:"supplierItemNo"`
	ModelNo        *string                     `json:"modelNo"`
	NameSuffix     *string                     `json:"nameSuffix"` // derived from the variant's axis values when empty
	StockStatus    domain.StockStatus          `json:"stockStatus"`
	IsDefault      bool                        `json:"isDefault"` // the first variant when none is marked
	Attributes     map[string]json.RawMessage  `json:"attributes"`
	Prices         map[domain.PriceTier]string `json:"prices"`
}

// validatedDoc is a product document that passed validation, with typed
// values and minted ids for new variants.
type validatedDoc struct {
	in       ProductInput
	schema   *categorySchema
	values   []domain.AttributeValue // product-level
	variants []validatedVariant
}

type validatedVariant struct {
	id     uuid.UUID
	isNew  bool
	in     VariantInput
	values []domain.AttributeValue
	prices map[domain.PriceTier]platform.Money
}

func isNull(raw json.RawMessage) bool {
	t := bytes.TrimSpace(raw)
	return len(t) == 0 || bytes.Equal(t, []byte("null"))
}

// sortedKeys gives deterministic iteration (and so a stable error order).
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// validateDocument checks a document against the category's attribute schema
// and the product rules. existing holds the ids of the product's current
// variants (nil on create). Required attributes are enforced only for active
// products: an incomplete product can be saved as a draft (or imported for
// review) but not published.
func validateDocument(ctx context.Context, q *store.Queries, in ProductInput, existing map[uuid.UUID]bool) (*validatedDoc, error) {
	v := &domain.ValidationError{}
	in.Name = requireText(v, "name", in.Name, maxProductNameLen)
	if in.Slug = strings.TrimSpace(in.Slug); in.Slug != "" {
		validateSlug(v, "slug", in.Slug)
	}
	in.Summary = optionalText(v, "summary", in.Summary, maxSummaryLen)
	in.Description = optionalText(v, "description", in.Description, maxProductDescLen)
	if in.Status == "" {
		in.Status = domain.StatusDraft
	} else if !in.Status.Valid() {
		v.Add("status", "must be draft, active, discontinued or needs_review")
	}
	publishing := in.Status == domain.StatusActive

	if in.BrandID != nil {
		if _, err := q.GetBrandByID(ctx, *in.BrandID); errors.Is(err, domain.ErrNotFound) {
			v.Add("brandId", "does not exist")
		} else if err != nil {
			return nil, fmt.Errorf("load brand: %w", err)
		}
	}

	doc := &validatedDoc{in: in}
	if _, err := q.GetCategoryByID(ctx, in.CategoryID); errors.Is(err, domain.ErrNotFound) {
		v.Add("categoryId", "does not exist")
		return nil, v // attributes cannot be checked without the category's schema
	} else if err != nil {
		return nil, fmt.Errorf("load category: %w", err)
	}
	var err error
	if doc.schema, err = loadCategorySchema(ctx, q, in.CategoryID); err != nil {
		return nil, err
	}

	// Product-level attributes.
	for _, key := range sortedKeys(in.Attributes) {
		raw, field := in.Attributes[key], "attributes."+key
		if isNull(raw) {
			continue
		}
		b := doc.schema.byKey[key]
		switch {
		case b == nil:
			v.Add(field, "is not an attribute of this category")
		case b.IsVariantAxis:
			v.Add(field, "varies by variant; set it on each variant")
		default:
			val, err := decodeValue(&b.Attribute, raw, nil)
			if err != nil {
				v.Add(field, "%s", err)
				continue
			}
			doc.values = append(doc.values, val)
		}
	}
	if publishing {
		for i := range doc.schema.bindings {
			b := &doc.schema.bindings[i]
			if b.IsRequired && !b.IsVariantAxis && isNull(in.Attributes[b.Attribute.Key]) {
				v.Add("attributes."+b.Attribute.Key, "is required to publish the product")
			}
		}
	}

	// Variants.
	switch n := len(in.Variants); {
	case n == 0:
		v.Add("variants", "at least one variant is required")
	case n > maxVariants:
		v.Add("variants", "at most %d variants are allowed", maxVariants)
	}
	defaults, seenIDs, seenSKUs := 0, map[uuid.UUID]bool{}, map[string]bool{}
	suppliers := map[uuid.UUID]bool{}
	for i, vi := range in.Variants {
		f := fmt.Sprintf("variants[%d]", i)
		vv := validatedVariant{in: vi, prices: map[domain.PriceTier]platform.Money{}}
		if vi.ID != nil {
			switch {
			case !existing[*vi.ID]:
				v.Add(f+".id", "is not a variant of this product")
			case seenIDs[*vi.ID]:
				v.Add(f+".id", "appears more than once")
			}
			seenIDs[*vi.ID] = true
			vv.id = *vi.ID
		} else {
			vv.id, vv.isNew = domain.NewID(), true
		}
		vv.in.SKU = requireText(v, f+".sku", vi.SKU, maxSKULen)
		if seenSKUs[vv.in.SKU] {
			v.Add(f+".sku", "duplicates another variant's SKU")
		}
		seenSKUs[vv.in.SKU] = true
		vv.in.SupplierItemNo = optionalText(v, f+".supplierItemNo", vi.SupplierItemNo, maxCodeLen)
		vv.in.ModelNo = optionalText(v, f+".modelNo", vi.ModelNo, maxCodeLen)
		vv.in.NameSuffix = optionalText(v, f+".nameSuffix", vi.NameSuffix, maxSuffixLen)
		if vi.SupplierID != nil && !suppliers[*vi.SupplierID] {
			if _, err := q.GetSupplierByID(ctx, *vi.SupplierID); errors.Is(err, domain.ErrNotFound) {
				v.Add(f+".supplierId", "does not exist")
			} else if err != nil {
				return nil, fmt.Errorf("load supplier: %w", err)
			}
			suppliers[*vi.SupplierID] = true
		}
		if vv.in.StockStatus == "" {
			vv.in.StockStatus = domain.StockUnknown
		} else if !vv.in.StockStatus.Valid() {
			v.Add(f+".stockStatus", "must be in_stock, low, out, special_order or unknown")
		}
		if vi.IsDefault {
			defaults++
		}

		for _, key := range sortedKeys(vi.Attributes) {
			raw, field := vi.Attributes[key], f+".attributes."+key
			if isNull(raw) {
				continue
			}
			b := doc.schema.byKey[key]
			switch {
			case b == nil:
				v.Add(field, "is not an attribute of this category")
			case !b.IsVariantAxis:
				v.Add(field, "describes the whole product; set it at product level")
			default:
				id := vv.id
				val, err := decodeValue(&b.Attribute, raw, &id)
				if err != nil {
					v.Add(field, "%s", err)
					continue
				}
				vv.values = append(vv.values, val)
			}
		}
		if publishing {
			for j := range doc.schema.bindings {
				b := &doc.schema.bindings[j]
				if b.IsRequired && b.IsVariantAxis && isNull(vi.Attributes[b.Attribute.Key]) {
					v.Add(f+".attributes."+b.Attribute.Key, "is required to publish the product")
				}
			}
		}

		for tier, amount := range vi.Prices {
			field := fmt.Sprintf("%s.prices.%s", f, tier)
			if !tier.Valid() {
				v.Add(field, "is not a price tier (cost_usd, landed_ttd, wholesale_ttd, retail_ttd)")
				continue
			}
			m, err := platform.ParseMoney(amount, tier.Currency())
			switch {
			case err != nil:
				v.Add(field, "must be a decimal amount with at most two decimal places")
			case m.AmountMinor() < 0:
				v.Add(field, "must not be negative")
			default:
				vv.prices[tier] = m
			}
		}
		if vv.in.NameSuffix == nil {
			vv.in.NameSuffix = derivedSuffix(doc.schema, vv.values)
		}
		doc.variants = append(doc.variants, vv)
	}
	if defaults > 1 {
		v.Add("variants", "only one variant can be the default")
	}
	if err := v.Err(); err != nil {
		return nil, err
	}
	if defaults == 0 {
		doc.variants[0].in.IsDefault = true
	}
	return doc, nil
}

// derivedSuffix names a variant after its axis values, e.g. "Black / 9/16"".
func derivedSuffix(s *categorySchema, values []domain.AttributeValue) *string {
	views := s.views(values)
	if len(views) == 0 {
		return nil
	}
	parts := make([]string, len(views))
	for i, vw := range views {
		parts[i] = vw.Display
	}
	suffix := strings.Join(parts, " / ")
	return &suffix
}

// Get returns a product's full admin view (every price tier, supplier data,
// unprocessed images).
func (s *Products) Get(ctx context.Context, id uuid.UUID) (*ProductView, error) {
	return getProductView(ctx, s.store.Queries, id)
}

// GetInTx is Get inside the caller's transaction.
func (s *Products) GetInTx(ctx context.Context, q *store.Queries, id uuid.UUID) (*ProductView, error) {
	return getProductView(ctx, q, id)
}

func getProductView(ctx context.Context, q *store.Queries, id uuid.UUID) (*ProductView, error) {
	row, err := q.GetProductByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("product %s: %w", id, err)
	}
	return loadProductView(ctx, q, store.Product(row), adminAudience)
}

// List returns one page of the admin product listing (any status).
func (s *Products) List(ctx context.Context, pq domain.ProductQuery) (domain.ProductPage, error) {
	cq, err := compileQuery(ctx, s.store.Queries, pq, false)
	if err != nil {
		return domain.ProductPage{}, err
	}
	return listProducts(ctx, s.store.Queries, cq, pq.Cursor)
}

// Create saves a new product document.
func (s *Products) Create(ctx context.Context, in ProductInput) (*ProductView, error) {
	var out *ProductView
	err := s.store.InTx(ctx, func(q *store.Queries) error {
		id, err := s.CreateInTx(ctx, q, in)
		if err != nil {
			return err
		}
		out, err = getProductView(ctx, q, id)
		return err
	})
	return out, err
}

// CreateInTx saves a new product document inside the caller's transaction and
// returns its id. The importer uses it to commit a whole batch atomically.
func (s *Products) CreateInTx(ctx context.Context, q *store.Queries, in ProductInput) (uuid.UUID, error) {
	doc, err := validateDocument(ctx, q, in, nil)
	if err != nil {
		return uuid.Nil, err
	}
	in = doc.in
	if in.Slug == "" {
		if in.Slug, err = uniqueSlug(ctx, in.Name, q.ProductSlugExists); err != nil {
			return uuid.Nil, err
		}
	}
	attrs, err := doc.schema.projection(doc.values)
	if err != nil {
		return uuid.Nil, err
	}
	var publishedAt *time.Time
	if in.Status == domain.StatusActive {
		now := time.Now()
		publishedAt = &now
	}
	row, err := q.CreateProduct(ctx, gen.CreateProductParams{
		ID:                  domain.NewID(),
		CategoryID:          in.CategoryID,
		BrandID:             in.BrandID,
		Name:                in.Name,
		Slug:                in.Slug,
		Summary:             store.Text(in.Summary),
		Description:         store.Text(in.Description),
		Status:              gen.ProductStatus(in.Status),
		IsFeatured:          in.IsFeatured,
		RetailPriceIsPublic: in.RetailPriceIsPublic,
		Attrs:               attrs,
		PublishedAt:         store.NullTimestamptz(publishedAt),
	})
	if err != nil {
		return uuid.Nil, err
	}
	if err := syncVariants(ctx, q, row.ID, doc); err != nil {
		return uuid.Nil, err
	}
	return row.ID, q.Audit(ctx, "product.create", "product", &row.ID, nil, in)
}

// Update replaces a product document. ifMatch, when given, must be the ETag
// the client last read; a concurrent save in between makes this fail with
// ErrVersionMismatch instead of silently overwriting the other edit.
func (s *Products) Update(ctx context.Context, id uuid.UUID, in ProductInput, ifMatch string) (*ProductView, error) {
	var out *ProductView
	err := s.store.InTx(ctx, func(q *store.Queries) error {
		if err := s.UpdateInTx(ctx, q, id, in, ifMatch); err != nil {
			return err
		}
		var err error
		out, err = getProductView(ctx, q, id)
		return err
	})
	return out, err
}

// UpdateInTx replaces a product document inside the caller's transaction.
func (s *Products) UpdateInTx(ctx context.Context, q *store.Queries, id uuid.UUID, in ProductInput, ifMatch string) error {
	before, err := getProductView(ctx, q, id)
	if err != nil {
		return err
	}
	cur := before.Product
	if err := checkIfMatch(ifMatch, cur.UpdatedAt); err != nil {
		return err
	}
	existing := make(map[uuid.UUID]bool, len(cur.Variants))
	for _, v := range cur.Variants {
		existing[v.ID] = true
	}
	doc, err := validateDocument(ctx, q, in, existing)
	if err != nil {
		return err
	}
	in = doc.in
	if in.Slug == "" {
		in.Slug = cur.Slug // URLs stay stable when a product is renamed
	}
	attrs, err := doc.schema.projection(doc.values)
	if err != nil {
		return err
	}
	publishedAt := cur.PublishedAt
	if in.Status == domain.StatusActive && publishedAt == nil {
		now := time.Now()
		publishedAt = &now
	}
	_, err = q.UpdateProduct(ctx, gen.UpdateProductParams{
		ID:                  id,
		CategoryID:          in.CategoryID,
		BrandID:             in.BrandID,
		Name:                in.Name,
		Slug:                in.Slug,
		Summary:             store.Text(in.Summary),
		Description:         store.Text(in.Description),
		Status:              gen.ProductStatus(in.Status),
		IsFeatured:          in.IsFeatured,
		RetailPriceIsPublic: in.RetailPriceIsPublic,
		Attrs:               attrs,
		PublishedAt:         store.NullTimestamptz(publishedAt),
		ExpectedUpdatedAt:   store.Timestamptz(cur.UpdatedAt),
	})
	if errors.Is(err, domain.ErrNotFound) {
		return domain.ErrVersionMismatch // it existed a moment ago: another save won the race
	}
	if err != nil {
		return err
	}
	if err := syncVariants(ctx, q, id, doc); err != nil {
		return err
	}
	return q.Audit(ctx, "product.update", "product", &id, before.Product, in)
}

// syncVariants makes the product's variants, attribute values and prices
// match the document. Order matters: removed variants go first (freeing
// their SKUs), the default flag is cleared before being reassigned (a unique
// index allows one default), and values are rewritten wholesale.
func syncVariants(ctx context.Context, q *store.Queries, productID uuid.UUID, doc *validatedDoc) error {
	keep := make([]uuid.UUID, 0, len(doc.variants))
	for _, v := range doc.variants {
		if !v.isNew {
			keep = append(keep, v.id)
		}
	}
	if err := q.DeleteVariantsExcept(ctx, gen.DeleteVariantsExceptParams{ProductID: productID, KeepIds: keep}); err != nil {
		return err
	}
	if err := q.ClearDefaultVariant(ctx, productID); err != nil {
		return err
	}
	if err := q.DeleteAttributeValuesForProduct(ctx, productID); err != nil {
		return err
	}
	current, err := currentPrices(ctx, q, productID)
	if err != nil {
		return err
	}

	values := slices.Clone(doc.values)
	for i, v := range doc.variants {
		attrs, err := doc.schema.projection(v.values)
		if err != nil {
			return err
		}
		if v.isNew {
			_, err = q.CreateVariant(ctx, gen.CreateVariantParams{
				ID: v.id, ProductID: productID, Sku: v.in.SKU, SupplierID: v.in.SupplierID,
				SupplierItemNo: store.Text(v.in.SupplierItemNo), ModelNo: store.Text(v.in.ModelNo),
				NameSuffix: store.Text(v.in.NameSuffix), Position: int32(i),
				StockStatus: gen.StockStatus(v.in.StockStatus), Attrs: attrs, IsDefault: v.in.IsDefault,
			})
		} else {
			_, err = q.UpdateVariant(ctx, gen.UpdateVariantParams{
				ID: v.id, ProductID: productID, Sku: v.in.SKU, SupplierID: v.in.SupplierID,
				SupplierItemNo: store.Text(v.in.SupplierItemNo), ModelNo: store.Text(v.in.ModelNo),
				NameSuffix: store.Text(v.in.NameSuffix), Position: int32(i),
				StockStatus: gen.StockStatus(v.in.StockStatus), Attrs: attrs, IsDefault: v.in.IsDefault,
			})
		}
		if err != nil {
			return fmt.Errorf("save variant %q: %w", v.in.SKU, err)
		}
		if err := applyPrices(ctx, q, v.id, v.prices, current); err != nil {
			return err
		}
		values = append(values, v.values...)
	}
	return insertValues(ctx, q, productID, values)
}

// UpdateVariantInTx records new prices, and a stock status unless it is
// empty, for one existing variant inside the caller's transaction. It backs
// the importer's "merge" of a re-imported price list; unchanged prices are
// not re-recorded.
func (s *Products) UpdateVariantInTx(ctx context.Context, q *store.Queries, productID, variantID uuid.UUID, prices map[domain.PriceTier]string, stock domain.StockStatus) error {
	variants, err := q.ListVariantsByProduct(ctx, productID)
	if err != nil {
		return fmt.Errorf("load variants: %w", err)
	}
	if !slices.ContainsFunc(variants, func(r gen.ProductVariant) bool { return r.ID == variantID }) {
		return fmt.Errorf("variant %s of product %s: %w", variantID, productID, domain.ErrNotFound)
	}
	v := &domain.ValidationError{}
	parsed := map[domain.PriceTier]platform.Money{}
	for tier, amount := range prices {
		m, err := platform.ParseMoney(amount, tier.Currency())
		if !tier.Valid() || err != nil || m.AmountMinor() < 0 {
			v.Add("prices."+string(tier), "must be a valid tier and a non-negative decimal amount")
			continue
		}
		parsed[tier] = m
	}
	if stock != "" && !stock.Valid() {
		v.Add("stockStatus", "is not a known stock status")
	}
	if err := v.Err(); err != nil {
		return err
	}
	current, err := currentPrices(ctx, q, productID)
	if err != nil {
		return err
	}
	if err := applyPrices(ctx, q, variantID, parsed, current); err != nil {
		return err
	}
	if stock != "" {
		if err := q.UpdateVariantStock(ctx, gen.UpdateVariantStockParams{ID: variantID, StockStatus: gen.StockStatus(stock)}); err != nil {
			return err
		}
	}
	if err := q.TouchProduct(ctx, productID); err != nil {
		return err
	}
	return q.Audit(ctx, "product.variant.update", "product", &productID, nil,
		map[string]any{"variantId": variantID, "prices": prices, "stockStatus": stock})
}

// currentPrices maps variant -> tier -> current price for one product.
func currentPrices(ctx context.Context, q *store.Queries, productID uuid.UUID) (map[uuid.UUID]map[domain.PriceTier]platform.Money, error) {
	rows, err := q.ListCurrentPricesByProduct(ctx, productID)
	if err != nil {
		return nil, fmt.Errorf("load current prices: %w", err)
	}
	out := map[uuid.UUID]map[domain.PriceTier]platform.Money{}
	for _, r := range rows {
		if out[r.VariantID] == nil {
			out[r.VariantID] = map[domain.PriceTier]platform.Money{}
		}
		out[r.VariantID][domain.PriceTier(r.Tier)] = platform.NewMoney(r.AmountMinor, r.Currency)
	}
	return out, nil
}

// applyPrices records each price that differs from the current one as a new
// row effective today, preserving the history of past prices.
func applyPrices(ctx context.Context, q *store.Queries, variantID uuid.UUID, prices map[domain.PriceTier]platform.Money, current map[uuid.UUID]map[domain.PriceTier]platform.Money) error {
	for tier, m := range prices {
		if cur, ok := current[variantID][tier]; ok && cur == m {
			continue
		}
		if _, err := q.UpsertPrice(ctx, gen.UpsertPriceParams{
			ID: domain.NewID(), VariantID: variantID, Tier: gen.PriceTier(tier),
			AmountMinor: m.AmountMinor(), Currency: m.Currency(),
		}); err != nil {
			return fmt.Errorf("save %s price: %w", tier, err)
		}
	}
	return nil
}

// insertValues writes EAV rows in one COPY. A multi_enum value becomes one
// row per selected option (the table's one-value-per-row check).
func insertValues(ctx context.Context, q *store.Queries, productID uuid.UUID, values []domain.AttributeValue) error {
	var rows []gen.InsertAttributeValuesParams
	for _, v := range values {
		base := gen.InsertAttributeValuesParams{
			ProductID: productID, VariantID: v.VariantID, AttributeID: v.AttributeID,
			ValueText: store.Text(v.Text), ValueNum: store.Float8(v.Num),
			ValueNumLow: store.Float8(v.NumLow), ValueNumHigh: store.Float8(v.NumHigh),
			ValueBool: store.Bool(v.Bool), Etrto: store.Text(v.ETRTO),
		}
		if len(v.OptionIDs) == 0 {
			base.ID = domain.NewID()
			rows = append(rows, base)
			continue
		}
		for _, opt := range v.OptionIDs {
			r := base
			r.ID, r.OptionID = domain.NewID(), &opt
			rows = append(rows, r)
		}
	}
	if len(rows) == 0 {
		return nil
	}
	if _, err := q.InsertAttributeValues(ctx, rows); err != nil {
		return fmt.Errorf("save attribute values: %w", err)
	}
	return nil
}

// rebuildProjections recomputes a product's JSONB projections from its EAV
// rows (after values were removed behind the product's back, e.g. an
// attribute unbound from its category).
func rebuildProjections(ctx context.Context, q *store.Queries, productID uuid.UUID, s *categorySchema) error {
	rows, err := q.ListAttributeValuesByProduct(ctx, productID)
	if err != nil {
		return fmt.Errorf("load attribute values: %w", err)
	}
	raw := make([]domain.AttributeValue, len(rows))
	for i, r := range rows {
		raw[i] = store.AttributeValue(r)
	}
	var productValues []domain.AttributeValue
	perVariant := map[uuid.UUID][]domain.AttributeValue{}
	for _, v := range mergeValues(raw) {
		if v.VariantID == nil {
			productValues = append(productValues, v)
		} else {
			perVariant[*v.VariantID] = append(perVariant[*v.VariantID], v)
		}
	}
	attrs, err := s.projection(productValues)
	if err != nil {
		return err
	}
	if err := q.UpdateProductAttrs(ctx, gen.UpdateProductAttrsParams{ID: productID, Attrs: attrs}); err != nil {
		return err
	}
	variants, err := q.ListVariantsByProduct(ctx, productID)
	if err != nil {
		return err
	}
	for _, v := range variants {
		attrs, err := s.projection(perVariant[v.ID])
		if err != nil {
			return err
		}
		if err := q.UpdateVariantAttrs(ctx, gen.UpdateVariantAttrsParams{ID: v.ID, Attrs: attrs}); err != nil {
			return err
		}
	}
	return nil
}

// Delete removes a product with its variants, prices and image attachments.
// Discontinuing (status "discontinued") is the reversible alternative.
func (s *Products) Delete(ctx context.Context, id uuid.UUID) error {
	return s.store.InTx(ctx, func(q *store.Queries) error {
		before, err := getProductView(ctx, q, id)
		if err != nil {
			return err
		}
		if _, err := q.DeleteProduct(ctx, id); err != nil {
			return err
		}
		return q.Audit(ctx, "product.delete", "product", &id, before.Product, nil)
	})
}

// ToInput converts an admin view back into a document, e.g. to append a
// variant to an existing product and save it.
func ToInput(v *ProductView) ProductInput {
	p := v.Product
	in := ProductInput{
		CategoryID: p.CategoryID, BrandID: p.BrandID, Name: p.Name, Slug: p.Slug,
		Summary: p.Summary, Description: p.Description, Status: p.Status,
		IsFeatured: p.IsFeatured, RetailPriceIsPublic: p.RetailPriceIsPublic,
		Attributes: rawValues(v.Attributes),
	}
	for _, variant := range p.Variants {
		id := variant.ID
		vi := VariantInput{
			ID: &id, SKU: variant.SKU, SupplierID: variant.SupplierID, SupplierItemNo: variant.SupplierItemNo,
			ModelNo: variant.ModelNo, NameSuffix: variant.NameSuffix, StockStatus: variant.StockStatus,
			IsDefault: variant.IsDefault, Attributes: rawValues(v.VariantAttributes[variant.ID]),
		}
		in.Variants = append(in.Variants, vi)
	}
	return in
}

func rawValues(views []AttributeView) map[string]json.RawMessage {
	out := make(map[string]json.RawMessage, len(views))
	for _, vw := range views {
		if b, err := json.Marshal(vw.Value); err == nil {
			out[vw.Key] = b
		}
	}
	return out
}

// ---- media attachments ------------------------------------------------------------

// MediaInput attaches an uploaded image to a product, optionally to one of
// its variants (a swatch must name its variant). AssetID is only used when
// attaching.
type MediaInput struct {
	AssetID   uuid.UUID        `json:"assetId"`
	VariantID *uuid.UUID       `json:"variantId"`
	Role      domain.MediaRole `json:"role"` // default gallery
	Position  int              `json:"position"`
	AltText   *string          `json:"altText"`
}

var mediaRoles = []domain.MediaRole{domain.MediaHero, domain.MediaGallery, domain.MediaDetail, domain.MediaSwatch}

func (in *MediaInput) validate(ctx context.Context, q *store.Queries, productID uuid.UUID) error {
	v := &domain.ValidationError{}
	if in.Role == "" {
		in.Role = domain.MediaGallery
	} else if !slices.Contains(mediaRoles, in.Role) {
		v.Add("role", "must be hero, gallery, detail or swatch")
	}
	if in.Role == domain.MediaSwatch && in.VariantID == nil {
		v.Add("variantId", "is required for a swatch")
	}
	if in.Position < 0 {
		v.Add("position", "must not be negative")
	}
	in.AltText = optionalText(v, "altText", in.AltText, maxAltTextLen)
	if in.VariantID != nil {
		variants, err := q.ListVariantsByProduct(ctx, productID)
		if err != nil {
			return err
		}
		if !slices.ContainsFunc(variants, func(r gen.ProductVariant) bool { return r.ID == *in.VariantID }) {
			v.Add("variantId", "is not a variant of this product")
		}
	}
	return v.Err()
}

// AttachMedia attaches an uploaded image to a product.
func (s *Products) AttachMedia(ctx context.Context, productID uuid.UUID, in MediaInput) (domain.ProductMedia, error) {
	var out domain.ProductMedia
	err := s.store.InTx(ctx, func(q *store.Queries) error {
		if _, err := q.GetProductByID(ctx, productID); err != nil {
			return fmt.Errorf("product %s: %w", productID, err)
		}
		if err := in.validate(ctx, q, productID); err != nil {
			return err
		}
		if _, err := q.GetAssetByID(ctx, in.AssetID); errors.Is(err, domain.ErrNotFound) {
			return domain.Invalid("assetId", "does not exist")
		} else if err != nil {
			return err
		}
		row, err := q.AttachProductMedia(ctx, gen.AttachProductMediaParams{
			ID: domain.NewID(), ProductID: productID, VariantID: in.VariantID, AssetID: in.AssetID,
			Role: gen.MediaRole(in.Role), Position: int32(in.Position), AltText: store.Text(in.AltText),
		})
		if err != nil {
			return err
		}
		out = store.ProductMedia(row)
		if err := q.TouchProduct(ctx, productID); err != nil {
			return err
		}
		return q.Audit(ctx, "product.media.attach", "product", &productID, nil, out)
	})
	return out, err
}

// UpdateMedia changes an attachment's role, variant, order or alt text.
func (s *Products) UpdateMedia(ctx context.Context, productID, mediaID uuid.UUID, in MediaInput) (domain.ProductMedia, error) {
	var out domain.ProductMedia
	err := s.store.InTx(ctx, func(q *store.Queries) error {
		if err := in.validate(ctx, q, productID); err != nil {
			return err
		}
		row, err := q.UpdateProductMedia(ctx, gen.UpdateProductMediaParams{
			ID: mediaID, ProductID: productID, VariantID: in.VariantID,
			Role: gen.MediaRole(in.Role), Position: int32(in.Position), AltText: store.Text(in.AltText),
		})
		if err != nil {
			return fmt.Errorf("product media %s: %w", mediaID, err)
		}
		out = store.ProductMedia(row)
		if err := q.TouchProduct(ctx, productID); err != nil {
			return err
		}
		return q.Audit(ctx, "product.media.update", "product", &productID, nil, out)
	})
	return out, err
}

// DetachMedia removes an image from a product (the upload itself remains).
func (s *Products) DetachMedia(ctx context.Context, productID, mediaID uuid.UUID) error {
	return s.store.InTx(ctx, func(q *store.Queries) error {
		n, err := q.DeleteProductMedia(ctx, gen.DeleteProductMediaParams{ID: mediaID, ProductID: productID})
		if err != nil {
			return err
		}
		if n == 0 {
			return fmt.Errorf("product media %s: %w", mediaID, domain.ErrNotFound)
		}
		if err := q.TouchProduct(ctx, productID); err != nil {
			return err
		}
		return q.Audit(ctx, "product.media.detach", "product", &productID, map[string]any{"mediaId": mediaID}, nil)
	})
}
