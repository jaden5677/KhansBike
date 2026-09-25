package importer

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/khansbikezone/bikezone-api/internal/domain"
	"github.com/khansbikezone/bikezone-api/internal/platform"
)

// Workbook layout. Each sheet holds one category (matched by name or slug).
// Somewhere in its first rows is a header row naming the columns. Columns are
// recognised by header, in any order: the core fields below by their usual
// spellings, and any other column by the key or label of an attribute bound
// to the sheet's category. Unrecognised columns are kept verbatim in the
// staged row's raw data and reported, never guessed at.

// Core fields a column can hold.
const (
	fieldName           = "name"
	fieldBrand          = "brand"
	fieldSKU            = "sku"
	fieldSupplierItemNo = "supplier_item_no"
	fieldModelNo        = "model_no"
	fieldSupplier       = "supplier"
	fieldSummary        = "summary"
	fieldStock          = "stock"
)

// headerSynonyms lists the header spellings recognised for each field,
// compared after platform.Slugify (so case, spacing and punctuation do not
// matter). Price tiers use the tier name as the field.
var headerSynonyms = map[string][]string{
	fieldName:                       {"name", "description", "item", "item-description", "item-name", "product", "product-name"},
	fieldBrand:                      {"brand", "make", "manufacturer"},
	fieldSKU:                        {"sku", "code", "item-code", "our-code", "stock-code"},
	fieldSupplierItemNo:             {"supplier-item-no", "supplier-item-number", "supplier-code", "item-no", "item-number", "part-no", "part-number"},
	fieldModelNo:                    {"model", "model-no", "model-number"},
	fieldSupplier:                   {"supplier", "vendor"},
	fieldSummary:                    {"summary", "notes", "details", "remarks"},
	fieldStock:                      {"stock", "stock-status", "qty", "quantity", "on-hand", "availability"},
	string(domain.TierCostUSD):      {"cost", "cost-usd", "usd", "usd-cost", "unit-cost", "fob"},
	string(domain.TierLandedTTD):    {"landed", "landed-cost", "landed-ttd", "landed-cost-ttd"},
	string(domain.TierWholesaleTTD): {"wholesale", "wholesale-ttd", "wholesale-price", "trade", "trade-price"},
	string(domain.TierRetailTTD):    {"retail", "retail-ttd", "retail-price", "price", "selling-price", "sell"},
}

var headerIndex = func() map[string]string {
	m := map[string]string{}
	for field, spellings := range headerSynonyms {
		for _, s := range spellings {
			m[s] = field
		}
	}
	return m
}()

// Limits mirrored from the product write path, so a problem surfaces during
// review rather than as a failed commit.
const (
	maxNameLen    = 200
	maxSKULen     = 64
	maxCodeLen    = 100
	maxSummaryLen = 500
	headerScan    = 10 // rows searched for the header
)

// column is one mapped header.
type column struct {
	header string
	field  string                    // a core field or price tier, or "" for an attribute/unknown column
	attr   *domain.CategoryAttribute // for attribute columns
}

// sheetContext is what parsing a sheet needs to know about the catalogue.
type sheetContext struct {
	category  *domain.Category
	bindings  []domain.CategoryAttribute
	brands    map[string]bool // lower-cased names of existing brands
	suppliers map[string]bool
}

// mapColumns names each header cell. Duplicate headers get a " (2)" suffix in
// the raw data so no cell is lost.
func mapColumns(header []string, bindings []domain.CategoryAttribute) []column {
	cols := make([]column, len(header))
	seen := map[string]int{}
	for i, h := range header {
		h = collapse(h)
		if h == "" {
			continue
		}
		if seen[h]++; seen[h] > 1 {
			h = fmt.Sprintf("%s (%d)", h, seen[h])
		}
		cols[i].header = h
		slug := platform.Slugify(h)
		if f, ok := headerIndex[slug]; ok {
			cols[i].field = f
			continue
		}
		for j := range bindings {
			b := &bindings[j]
			if slug == platform.Slugify(b.Attribute.Key) || slug == platform.Slugify(b.Attribute.Label) ||
				slug == platform.Slugify(b.EffectiveLabel()) {
				cols[i].attr = b
				break
			}
		}
	}
	return cols
}

// findHeader locates the header row: the first row (among the first few) that
// has a name column and at least one other recognised column.
func findHeader(rows [][]string, bindings []domain.CategoryAttribute) (int, []column, bool) {
	for i := 0; i < len(rows) && i < headerScan; i++ {
		cols := mapColumns(rows[i], bindings)
		hasName, recognised := false, 0
		for _, c := range cols {
			if c.field == fieldName {
				hasName = true
			}
			if c.field != "" || c.attr != nil {
				recognised++
			}
		}
		if hasName && recognised >= 2 {
			return i, cols, true
		}
	}
	return 0, nil, false
}

// stagedRow is one source row on its way into import_rows.
type stagedRow struct {
	sheet    string
	index    int
	raw      map[string]string
	proposal domain.ImportProposal
	issues   []domain.ImportIssue
	decision domain.ImportDecision
	target   *uuid.UUID
}

func (r *stagedRow) add(code, severity, field, format string, args ...any) {
	r.issues = append(r.issues, domain.ImportIssue{Code: code, Severity: severity, Field: field, Message: fmt.Sprintf(format, args...)})
}

func (r *stagedRow) has(severity string) bool {
	for _, is := range r.issues {
		if is.Severity == severity {
			return true
		}
	}
	return false
}

// sheetProblem stages a single placeholder row that explains why a whole
// sheet cannot be imported. It is skipped automatically but stays visible in
// the review.
func sheetProblem(sheet, code, format string, args ...any) *stagedRow {
	r := &stagedRow{sheet: sheet, index: 0, raw: map[string]string{}, decision: domain.DecisionSkip}
	r.add(code, domain.SeverityError, "", format, args...)
	return r
}

// parseSheet stages every data row of a sheet.
func parseSheet(sheet string, rows [][]string, sc *sheetContext) []*stagedRow {
	headerAt, cols, ok := findHeader(rows, sc.bindings)
	if !ok {
		return []*stagedRow{sheetProblem(sheet, "no_header_row",
			"no header row with a name/description column was found in the first %d rows", headerScan)}
	}
	var unmapped []string
	for _, c := range cols {
		if c.header != "" && c.field == "" && c.attr == nil {
			unmapped = append(unmapped, c.header)
		}
	}
	var out []*stagedRow
	for i := headerAt + 1; i < len(rows); i++ {
		r := parseRow(sheet, i, rows[i], cols, sc)
		if r == nil {
			continue // blank row
		}
		if len(out) == 0 && len(unmapped) > 0 {
			r.add("unmapped_columns", domain.SeverityInfo, "",
				"these columns match no field or attribute of %s and were not imported: %s", sc.category.Name, strings.Join(unmapped, ", "))
		}
		out = append(out, r)
	}
	return out
}

// parseRow reads one data row into a proposal plus issues; nil for a blank row.
func parseRow(sheet string, index int, cells []string, cols []column, sc *sheetContext) *stagedRow {
	r := &stagedRow{sheet: sheet, index: index, raw: map[string]string{}, decision: domain.DecisionAccept}
	p := &r.proposal
	p.CategoryID = sc.category.ID
	p.Status = domain.StatusActive
	p.StockStatus = domain.StockUnknown
	p.Prices = map[domain.PriceTier]string{}
	p.ProductAttributes = map[string]json.RawMessage{}
	p.VariantAttributes = map[string]json.RawMessage{}

	for i, col := range cols {
		if i >= len(cells) || strings.TrimSpace(cells[i]) == "" || col.header == "" {
			continue
		}
		cell := cells[i]
		r.raw[col.header] = cell
		switch {
		case col.attr != nil:
			r.parseAttribute(col, cell)
		case col.field != "":
			r.parseField(col, cell, sc)
		}
	}
	if len(r.raw) == 0 {
		return nil
	}

	if p.Name == "" {
		r.add("missing_name", domain.SeverityError, "name", "the row has no name/description")
	} else if utf8.RuneCountInString(p.Name) > maxNameLen {
		r.add("too_long", domain.SeverityError, "name", "the name is longer than %d characters", maxNameLen)
	}
	if utf8.RuneCountInString(p.Summary) > maxSummaryLen {
		r.add("too_long", domain.SeverityError, "summary", "the summary is longer than %d characters", maxSummaryLen)
	}
	for field, v := range map[string]string{"sku": p.SKU, "supplierItemNo": p.SupplierItemNo, "modelNo": p.ModelNo} {
		limit := maxCodeLen
		if field == "sku" {
			limit = maxSKULen
		}
		if len(v) > limit {
			r.add("too_long", domain.SeverityError, field, "%q is longer than %d characters", v, limit)
		}
	}
	if _, ok := p.Prices[domain.TierRetailTTD]; !ok && !r.hasIssueOn(string(domain.TierRetailTTD)) {
		r.add("missing_retail_price", domain.SeverityWarning, "prices.retail_ttd", "the row has no retail price")
	}
	for _, b := range sc.bindings {
		if !b.IsRequired {
			continue
		}
		set := p.ProductAttributes
		if b.IsVariantAxis {
			set = p.VariantAttributes
		}
		if _, ok := set[b.Attribute.Key]; !ok && !r.hasIssueOn(b.Attribute.Key) {
			r.add("missing_required_attribute", domain.SeverityWarning, b.Attribute.Key,
				"%s is required for %s but the row has no value; it will be imported for review", b.EffectiveLabel(), sc.category.Name)
		}
	}
	if p.SKU == "" && p.Name != "" {
		p.SKU = generatedSKU(p)
		r.add("generated_sku", domain.SeverityInfo, "sku", "no SKU column; generated %q", p.SKU)
	}
	return r
}

func (r *stagedRow) hasIssueOn(field string) bool {
	for _, is := range r.issues {
		if is.Field == field || strings.HasSuffix(is.Field, "."+field) {
			return true
		}
	}
	return false
}

func (r *stagedRow) parseField(col column, cell string, sc *sheetContext) {
	p := &r.proposal
	value := collapse(cell)
	switch col.field {
	case fieldName:
		p.Name = value
	case fieldBrand:
		p.Brand = value
		if value != cell {
			r.add("brand_whitespace", domain.SeverityInfo, "brand", "brand %q had stray spaces; read as %q", cell, value)
		}
		if !sc.brands[strings.ToLower(value)] {
			r.add("new_brand", domain.SeverityInfo, "brand", "brand %q does not exist yet and will be created", value)
		}
	case fieldSupplier:
		p.Supplier = value
		if !sc.suppliers[strings.ToLower(value)] {
			r.add("new_supplier", domain.SeverityInfo, "supplier", "supplier %q does not exist yet and will be created", value)
		}
	case fieldSKU:
		p.SKU = value
	case fieldSupplierItemNo:
		p.SupplierItemNo = value
	case fieldModelNo:
		p.ModelNo = value
	case fieldSummary:
		p.Summary = value
	case fieldStock:
		status, ok := parseStock(cell)
		p.StockStatus = status
		if !ok {
			r.add("unknown_stock", domain.SeverityInfo, "stockStatus", "stock %q was not understood; recorded as unknown", cell)
		}
	default: // a price tier
		tier := domain.PriceTier(col.field)
		field := "prices." + col.field
		m, rounded, err := platform.ParseMoneyRounded(cell, tier.Currency())
		switch {
		case err != nil:
			r.add("invalid_price", domain.SeverityError, field, "%s %q is not a number", col.header, cell)
		case m.AmountMinor() < 0:
			r.add("invalid_price", domain.SeverityError, field, "%s %q is negative", col.header, cell)
		default:
			p.Prices[tier] = m.Decimal()
			if rounded {
				r.add("price_rounded", domain.SeverityInfo, field, "%s %q was rounded to %s", col.header, cell, m.Decimal())
			}
		}
	}
}

func (r *stagedRow) parseAttribute(col column, cell string) {
	b := col.attr
	v, err := parseAttributeCell(&b.Attribute, cell)
	if err != nil {
		code := "invalid_attribute_value"
		if b.Attribute.DataType.UsesOptions() {
			code = "unknown_option"
		}
		r.add(code, domain.SeverityError, b.Attribute.Key, "%s", err)
		return
	}
	if b.IsVariantAxis {
		r.proposal.VariantAttributes[b.Attribute.Key] = v
	} else {
		r.proposal.ProductAttributes[b.Attribute.Key] = v
	}
}

// generatedSKU derives a deterministic SKU from the product and its variant
// values, so re-importing the same sheet produces the same SKUs and matches
// the products the first import created.
func generatedSKU(p *domain.ImportProposal) string {
	parts := []string{p.Brand, p.Name}
	keys := make([]string, 0, len(p.VariantAttributes))
	for k := range p.VariantAttributes {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		parts = append(parts, string(p.VariantAttributes[k]))
	}
	sku := strings.ToUpper(platform.Slugify(strings.Join(parts, " ")))
	if len(sku) > maxSKULen {
		sku = strings.TrimRight(sku[:maxSKULen], "-")
	}
	return sku
}

// ---- grouping and duplicates --------------------------------------------------

// groupKey identifies the product a row belongs to: rows of one category
// with the same name, brand and product-level attributes are variants of one
// product. json.Marshal sorts map keys, so the encoding is canonical.
func groupKey(p *domain.ImportProposal) string {
	attrs, _ := json.Marshal(p.ProductAttributes)
	return strings.Join([]string{p.CategoryID.String(), strings.ToLower(p.Name), strings.ToLower(p.Brand), string(attrs)}, "|")
}

// signature is everything that describes the variant a row stands for; two
// rows with equal signatures are the same line entered twice.
func signature(p *domain.ImportProposal) string {
	b, _ := json.Marshal(struct {
		SKU, SupplierItemNo, ModelNo string
		Stock                        domain.StockStatus
		Attrs                        map[string]json.RawMessage
		Prices                       map[domain.PriceTier]string
	}{p.SKU, p.SupplierItemNo, p.ModelNo, p.StockStatus, p.VariantAttributes, p.Prices})
	return string(b)
}

// groupRows assigns group keys and resolves duplicates. An exact duplicate is
// skipped and its first occurrence flagged, so the product is imported once,
// for review (the workbook's six identical helmet rows, whose colours were
// never recorded, become one product in needs_review rather than six products
// or a silent merge). Rows that collide on SKU without being identical keep
// distinct SKUs and are flagged too. Any warning in a group sends the whole
// product to review instead of publishing it.
func groupRows(rows []*stagedRow) {
	var order []string
	groups := map[string][]*stagedRow{}
	for _, r := range rows {
		if r.has(domain.SeverityError) || r.decision == domain.DecisionSkip {
			continue
		}
		r.proposal.GroupKey = groupKey(&r.proposal)
		if _, ok := groups[r.proposal.GroupKey]; !ok {
			order = append(order, r.proposal.GroupKey)
		}
		groups[r.proposal.GroupKey] = append(groups[r.proposal.GroupKey], r)
	}
	for _, key := range order {
		members := groups[key]
		firstBySig := map[string]*stagedRow{}
		flagged := map[*stagedRow]bool{}
		skus := map[string]int{}
		for _, r := range members {
			sig := signature(&r.proposal)
			if first, dup := firstBySig[sig]; dup {
				r.decision = domain.DecisionSkip
				r.add("duplicate_row", domain.SeverityWarning, "", "identical to row %d of sheet %q; skipped", first.index+1, first.sheet)
				if !flagged[first] {
					flagged[first] = true
					first.add("has_duplicates", domain.SeverityWarning, "",
						"this row appears more than once; check whether the copies differ in something the sheet does not record (such as colour)")
				}
				continue
			}
			firstBySig[sig] = r
			if n := skus[r.proposal.SKU]; n > 0 {
				renamed := fmt.Sprintf("%s-%d", r.proposal.SKU, n+1)
				r.add("ambiguous_variant", domain.SeverityWarning, "sku",
					"another row of this product has SKU %q; renamed to %q", r.proposal.SKU, renamed)
				r.proposal.SKU = renamed
			}
			skus[r.proposal.SKU]++
		}
		review := false
		for _, r := range members {
			if r.decision != domain.DecisionSkip && r.has(domain.SeverityWarning) {
				review = true
			}
		}
		if review {
			for _, r := range members {
				r.proposal.Status = domain.StatusNeedsReview
			}
		}
	}
}
