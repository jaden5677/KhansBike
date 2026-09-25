// Package importer brings the owner's price-list workbook (.xlsx) into the
// catalogue in two steps. Staging parses the workbook into import_rows
// without touching the catalogue: every row gets a normalised proposal, the
// issues found in it (typos, duplicates, missing prices, float noise...) and a
// suggested decision. A person reviews and adjusts the decisions; committing
// then applies the accepted rows in one transaction through the same product
// write path the admin API uses, so imported data obeys the same rules.
//
// Re-importing an updated workbook is safe: rows whose SKU or supplier item
// number matches an existing variant are proposed as "merge" (update that
// variant's prices and stock) rather than as new products.
package importer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/google/uuid"
	"github.com/xuri/excelize/v2"

	"github.com/khansbikezone/bikezone-api/internal/domain"
	"github.com/khansbikezone/bikezone-api/internal/platform"
	"github.com/khansbikezone/bikezone-api/internal/service"
	"github.com/khansbikezone/bikezone-api/internal/store"
	"github.com/khansbikezone/bikezone-api/internal/store/gen"
)

// MaxWorkbookBytes bounds an uploaded workbook.
const MaxWorkbookBytes = 32 << 20

// Importer stages, reviews and commits workbook imports.
type Importer struct {
	store    *store.Store
	taxonomy *service.Taxonomy
	products *service.Products
}

// New builds the importer.
func New(st *store.Store, taxonomy *service.Taxonomy, products *service.Products) *Importer {
	return &Importer{store: st, taxonomy: taxonomy, products: products}
}

// Stage parses a workbook into a new dry-run batch.
func (im *Importer) Stage(ctx context.Context, filename string, body io.Reader) (domain.ImportBatch, error) {
	data, err := io.ReadAll(io.LimitReader(body, MaxWorkbookBytes+1))
	if err != nil {
		return domain.ImportBatch{}, fmt.Errorf("receive workbook: %w", err)
	}
	if len(data) > MaxWorkbookBytes {
		return domain.ImportBatch{}, domain.Invalid("file", "is larger than the %d byte limit", MaxWorkbookBytes)
	}
	wb, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return domain.ImportBatch{}, domain.Invalid("file", "is not a readable .xlsx workbook")
	}
	defer func() { _ = wb.Close() }()

	rows, err := im.parseWorkbook(ctx, wb)
	if err != nil {
		return domain.ImportBatch{}, err
	}
	groupRows(rows)
	if err := im.matchExisting(ctx, rows); err != nil {
		return domain.ImportBatch{}, err
	}
	for _, r := range rows {
		if r.has(domain.SeverityError) && r.decision != domain.DecisionSkip {
			r.decision = domain.DecisionPending // a person must decide
		}
	}

	sum := sha256.Sum256(data)
	filename = strings.TrimSpace(filename)
	if filename == "" {
		filename = "workbook.xlsx"
	}
	if len(filename) > 255 {
		filename = filename[:255]
	}
	var batch domain.ImportBatch
	err = im.store.InTx(ctx, func(q *store.Queries) error {
		b, err := q.CreateImportBatch(ctx, gen.CreateImportBatchParams{
			ID: domain.NewID(), Filename: filename, Sha256: sum[:], CreatedBy: domain.ActorFrom(ctx).UserID,
		})
		if err != nil {
			return err
		}
		batch = store.ImportBatch(b)
		params := make([]gen.InsertImportRowsParams, len(rows))
		for i, r := range rows {
			if params[i], err = r.params(batch.ID); err != nil {
				return err
			}
		}
		if len(params) > 0 {
			if _, err := q.InsertImportRows(ctx, params); err != nil {
				return fmt.Errorf("stage rows: %w", err)
			}
		}
		return q.Audit(ctx, "import.stage", "import_batch", &batch.ID, nil,
			map[string]any{"filename": filename, "rows": len(rows)})
	})
	if err != nil {
		return domain.ImportBatch{}, err
	}
	return im.Batch(ctx, batch.ID)
}

func (r *stagedRow) params(batchID uuid.UUID) (gen.InsertImportRowsParams, error) {
	raw, err := json.Marshal(r.raw)
	if err != nil {
		return gen.InsertImportRowsParams{}, err
	}
	proposed, err := json.Marshal(r.proposal)
	if err != nil {
		return gen.InsertImportRowsParams{}, err
	}
	if r.issues == nil {
		r.issues = []domain.ImportIssue{}
	}
	issues, err := json.Marshal(r.issues)
	if err != nil {
		return gen.InsertImportRowsParams{}, err
	}
	return gen.InsertImportRowsParams{
		ID: domain.NewID(), BatchID: batchID, SheetName: r.sheet, RowIndex: int32(r.index),
		Raw: raw, Proposed: proposed, Issues: issues,
		Decision: gen.ImportDecision(r.decision), TargetProductID: r.target,
	}, nil
}

// parseWorkbook stages every sheet, matching each to a category by name.
func (im *Importer) parseWorkbook(ctx context.Context, wb *excelize.File) ([]*stagedRow, error) {
	categories, err := im.taxonomy.ListCategories(ctx)
	if err != nil {
		return nil, err
	}
	bySlug := map[string]*domain.Category{}
	for i := range categories {
		c := &categories[i]
		bySlug[c.Slug] = c
		bySlug[platform.Slugify(c.Name)] = c
	}
	known := func(names []string) map[string]bool {
		m := make(map[string]bool, len(names))
		for _, n := range names {
			m[strings.ToLower(n)] = true
		}
		return m
	}
	brands, err := im.taxonomy.ListBrands(ctx)
	if err != nil {
		return nil, err
	}
	suppliers, err := im.taxonomy.ListSuppliers(ctx)
	if err != nil {
		return nil, err
	}
	brandNames := make([]string, len(brands))
	for i, b := range brands {
		brandNames[i] = b.Name
	}
	supplierNames := make([]string, len(suppliers))
	for i, s := range suppliers {
		supplierNames[i] = s.Name
	}

	var rows []*stagedRow
	for _, sheet := range wb.GetSheetList() {
		cat, ok := bySlug[platform.Slugify(sheet)]
		if !ok {
			rows = append(rows, sheetProblem(sheet, "unknown_sheet",
				"no category is named %q; create it (or rename the sheet) and import again", sheet))
			continue
		}
		bindings, err := im.taxonomy.CategoryAttributes(ctx, cat.ID)
		if err != nil {
			return nil, err
		}
		// Raw values, not the cell's display format: prices keep their full
		// precision (then get rounded explicitly, with an issue noting it).
		cells, err := wb.GetRows(sheet, excelize.Options{RawCellValue: true})
		if err != nil {
			return nil, domain.Invalid("file", "sheet %q cannot be read: %s", sheet, err)
		}
		rows = append(rows, parseSheet(sheet, cells, &sheetContext{
			category: cat, bindings: bindings, brands: known(brandNames), suppliers: known(supplierNames),
		})...)
	}
	return rows, nil
}

// matchExisting proposes a merge for rows whose SKU or supplier item number
// identifies exactly one existing variant, and points the other accepted rows
// of such a product at it, so re-importing a sheet that gained a colour adds
// a variant to the existing product instead of creating a duplicate.
func (im *Importer) matchExisting(ctx context.Context, rows []*stagedRow) error {
	codes := map[string]bool{}
	for _, r := range rows {
		if r.decision == domain.DecisionAccept {
			for _, c := range []string{r.proposal.SKU, r.proposal.SupplierItemNo} {
				if c != "" {
					codes[c] = true
				}
			}
		}
	}
	if len(codes) == 0 {
		return nil
	}
	list := make([]string, 0, len(codes))
	for c := range codes {
		list = append(list, c)
	}
	variants, err := im.store.ListVariantsByCodes(ctx, list)
	if err != nil {
		return fmt.Errorf("match existing variants: %w", err)
	}

	groupTarget := map[string]uuid.UUID{}
	for _, r := range rows {
		if r.decision != domain.DecisionAccept {
			continue
		}
		p := &r.proposal
		var matches []gen.ListVariantsByCodesRow
		for _, v := range variants {
			bySKU := v.Sku == p.SKU
			byItem := p.SupplierItemNo != "" && (v.SupplierItemNo.String == p.SupplierItemNo || v.Sku == p.SupplierItemNo)
			if bySKU || byItem {
				matches = append(matches, v)
			}
		}
		switch len(matches) {
		case 0:
		case 1:
			m := matches[0]
			r.decision, r.target, p.TargetVariantID = domain.DecisionMerge, &m.ProductID, &m.ID
			r.add("matches_existing", domain.SeverityInfo, "sku",
				"matches existing variant %q; committing updates its prices and stock", m.Sku)
			groupTarget[p.GroupKey] = m.ProductID
		default:
			r.decision = domain.DecisionPending
			r.add("ambiguous_match", domain.SeverityWarning, "sku",
				"%d existing variants share this SKU or supplier item number; decide whether to create a new product or skip", len(matches))
		}
	}
	for _, r := range rows {
		if target, ok := groupTarget[r.proposal.GroupKey]; ok && r.decision == domain.DecisionAccept {
			r.target = &target
			r.add("adds_variant", domain.SeverityInfo, "", "will be added as a new variant of the existing product")
		}
	}
	return nil
}

// Batch returns a batch with its row counts per decision.
func (im *Importer) Batch(ctx context.Context, id uuid.UUID) (domain.ImportBatch, error) {
	return batch(ctx, im.store.Queries, id)
}

func batch(ctx context.Context, q *store.Queries, id uuid.UUID) (domain.ImportBatch, error) {
	row, err := q.GetImportBatch(ctx, id)
	if err != nil {
		return domain.ImportBatch{}, fmt.Errorf("import batch %s: %w", id, err)
	}
	b := store.ImportBatch(row)
	counts, err := q.CountImportRowsByDecision(ctx, id)
	if err != nil {
		return domain.ImportBatch{}, fmt.Errorf("count import rows: %w", err)
	}
	b.Counts = map[domain.ImportDecision]int{
		domain.DecisionPending: 0, domain.DecisionAccept: 0, domain.DecisionSkip: 0, domain.DecisionMerge: 0,
	}
	for _, c := range counts {
		b.Counts[domain.ImportDecision(c.Decision)] = int(c.Total)
	}
	return b, nil
}

// Batches lists the most recent batches (without row counts).
func (im *Importer) Batches(ctx context.Context) ([]domain.ImportBatch, error) {
	rows, err := im.store.ListImportBatches(ctx, 50)
	if err != nil {
		return nil, fmt.Errorf("list import batches: %w", err)
	}
	out := make([]domain.ImportBatch, len(rows))
	for i, r := range rows {
		out[i] = store.ImportBatch(r)
	}
	return out, nil
}

// RowQuery narrows the review list.
type RowQuery struct {
	Decision       domain.ImportDecision // "" = any
	WithIssuesOnly bool
	Cursor         string
	Limit          int
}

// Rows returns one page of a batch's staged rows, in workbook order.
func (im *Importer) Rows(ctx context.Context, batchID uuid.UUID, rq RowQuery) (rows []domain.ImportRow, next string, err error) {
	if rq.Decision != "" && !rq.Decision.Valid() {
		return nil, "", domain.Invalid("decision", "must be pending, accept, skip or merge")
	}
	if _, err := im.store.GetImportBatch(ctx, batchID); err != nil {
		return nil, "", fmt.Errorf("import batch %s: %w", batchID, err)
	}
	limit := platform.ClampLimit(rq.Limit, 100, 500)
	after := uuid.Nil
	if rq.Cursor != "" {
		if after, err = uuid.Parse(rq.Cursor); err != nil {
			return nil, "", domain.Invalid("cursor", "is invalid")
		}
	}
	params := gen.ListImportRowsParams{BatchID: batchID, WithIssuesOnly: rq.WithIssuesOnly, AfterID: after, RowLimit: int32(limit + 1)}
	if rq.Decision != "" {
		params.Decision = gen.NullImportDecision{ImportDecision: gen.ImportDecision(rq.Decision), Valid: true}
	}
	list, err := im.store.ListImportRows(ctx, params)
	if err != nil {
		return nil, "", fmt.Errorf("list import rows: %w", err)
	}
	for _, r := range list {
		row, err := store.ImportRow(r)
		if err != nil {
			return nil, "", err
		}
		rows = append(rows, row)
	}
	if len(rows) > limit {
		rows = rows[:limit]
		next = rows[limit-1].ID.String()
	}
	return rows, next, nil
}

var errBatchClosed = fmt.Errorf("%w: the import batch has already been committed or aborted", domain.ErrConflict)

// SetDecision records a reviewer's decision for one row.
func (im *Importer) SetDecision(ctx context.Context, batchID, rowID uuid.UUID, d domain.ImportDecision) (domain.ImportRow, error) {
	if !d.Valid() {
		return domain.ImportRow{}, domain.Invalid("decision", "must be pending, accept, skip or merge")
	}
	var out domain.ImportRow
	err := im.store.InTx(ctx, func(q *store.Queries) error {
		b, err := q.GetImportBatch(ctx, batchID)
		if err != nil {
			return fmt.Errorf("import batch %s: %w", batchID, err)
		}
		if !domain.ImportBatchStatus(b.Status).IsOpen() {
			return errBatchClosed
		}
		row, err := q.GetImportRow(ctx, gen.GetImportRowParams{ID: rowID, BatchID: batchID})
		if err != nil {
			return fmt.Errorf("import row %s: %w", rowID, err)
		}
		cur, err := store.ImportRow(row)
		if err != nil {
			return err
		}
		switch {
		case d == domain.DecisionAccept && cur.HasErrors():
			return domain.Invalid("decision", "the row has errors; fix the workbook and import it again, or skip the row")
		case d == domain.DecisionMerge && cur.Proposed.TargetVariantID == nil:
			return domain.Invalid("decision", "the row matches no existing variant to merge into")
		}
		updated, err := q.SetImportRowDecision(ctx, gen.SetImportRowDecisionParams{ID: rowID, BatchID: batchID, Decision: gen.ImportDecision(d)})
		if err != nil {
			return err
		}
		if out, err = store.ImportRow(updated); err != nil {
			return err
		}
		_, err = q.SetImportBatchStatus(ctx, gen.SetImportBatchStatusParams{
			ID: batchID, ToStatus: string(domain.ImportReviewing), FromStatuses: []string{string(domain.ImportDryRun)},
		})
		return err
	})
	return out, err
}

// Abort closes a batch without applying it.
func (im *Importer) Abort(ctx context.Context, batchID uuid.UUID) error {
	return im.store.InTx(ctx, func(q *store.Queries) error {
		if _, err := q.GetImportBatch(ctx, batchID); err != nil {
			return fmt.Errorf("import batch %s: %w", batchID, err)
		}
		n, err := q.SetImportBatchStatus(ctx, gen.SetImportBatchStatusParams{
			ID: batchID, ToStatus: string(domain.ImportAborted),
			FromStatuses: []string{string(domain.ImportDryRun), string(domain.ImportReviewing)},
		})
		if err != nil {
			return err
		}
		if n == 0 {
			return errBatchClosed
		}
		return q.Audit(ctx, "import.abort", "import_batch", &batchID, nil, nil)
	})
}

// CommitOptions are the choices made when applying a batch.
type CommitOptions struct {
	// RetailPricesPublic publishes the retail price of the products this
	// import creates. Off by default: showing prices is an explicit choice.
	RetailPricesPublic bool `json:"retailPricesPublic"`
}

// Commit applies every accepted and merged row of a batch in a single
// transaction: either the whole batch lands or nothing does. Rows still
// pending block the commit.
func (im *Importer) Commit(ctx context.Context, batchID uuid.UUID, opts CommitOptions) (domain.ImportBatch, error) {
	err := im.store.InTx(ctx, func(q *store.Queries) error {
		b, err := batch(ctx, q, batchID)
		if err != nil {
			return err
		}
		if !b.Status.IsOpen() {
			return errBatchClosed
		}
		if n := b.Counts[domain.DecisionPending]; n > 0 {
			return domain.Invalid("decisions", "%d rows still need a decision (accept, merge or skip)", n)
		}
		n, err := q.SetImportBatchStatus(ctx, gen.SetImportBatchStatusParams{
			ID: batchID, ToStatus: string(domain.ImportCommitted),
			FromStatuses: []string{string(domain.ImportDryRun), string(domain.ImportReviewing)},
		})
		if err != nil {
			return err
		}
		if n == 0 {
			return errBatchClosed // a concurrent commit or abort won
		}
		c := &committer{im: im, q: q, opts: opts, brands: map[string]uuid.UUID{}, suppliers: map[string]uuid.UUID{}}
		if err := c.load(ctx); err != nil {
			return err
		}
		all, err := q.ListAllImportRows(ctx, batchID)
		if err != nil {
			return err
		}
		created, updated, err := c.apply(ctx, all)
		if err != nil {
			return err
		}
		return q.Audit(ctx, "import.commit", "import_batch", &batchID, nil,
			map[string]any{"productsCreated": created, "rowsApplied": updated, "retailPricesPublic": opts.RetailPricesPublic})
	})
	if err != nil {
		return domain.ImportBatch{}, err
	}
	return im.Batch(ctx, batchID)
}

// committer applies one batch inside the commit transaction.
type committer struct {
	im        *Importer
	q         *store.Queries
	opts      CommitOptions
	brands    map[string]uuid.UUID // lower-cased name -> id
	suppliers map[string]uuid.UUID
}

func (c *committer) load(ctx context.Context) error {
	brands, err := c.q.ListBrands(ctx)
	if err != nil {
		return err
	}
	for _, b := range brands {
		c.brands[strings.ToLower(b.Name)] = b.ID
	}
	suppliers, err := c.q.ListSuppliers(ctx)
	if err != nil {
		return err
	}
	for _, s := range suppliers {
		c.suppliers[strings.ToLower(s.Name)] = s.ID
	}
	return nil
}

func (c *committer) brandID(ctx context.Context, name string) (*uuid.UUID, error) {
	if name == "" {
		return nil, nil
	}
	if id, ok := c.brands[strings.ToLower(name)]; ok {
		return &id, nil
	}
	b, err := c.im.taxonomy.CreateBrandInTx(ctx, c.q, service.BrandInput{Name: name})
	if err != nil {
		return nil, fmt.Errorf("create brand %q: %w", name, err)
	}
	c.brands[strings.ToLower(name)] = b.ID
	return &b.ID, nil
}

func (c *committer) supplierID(ctx context.Context, name string) (*uuid.UUID, error) {
	if name == "" {
		return nil, nil
	}
	if id, ok := c.suppliers[strings.ToLower(name)]; ok {
		return &id, nil
	}
	s, err := c.im.taxonomy.CreateSupplierInTx(ctx, c.q, service.SupplierInput{Name: name})
	if err != nil {
		return nil, fmt.Errorf("create supplier %q: %w", name, err)
	}
	c.suppliers[strings.ToLower(name)] = s.ID
	return &s.ID, nil
}

// apply creates or extends a product per group and merges matched rows.
func (c *committer) apply(ctx context.Context, all []gen.ImportRow) (created, applied int, err error) {
	var order []string
	groups := map[string][]domain.ImportRow{}
	for _, r := range all {
		row, err := store.ImportRow(r)
		if err != nil {
			return 0, 0, err
		}
		if row.Decision != domain.DecisionAccept && row.Decision != domain.DecisionMerge {
			continue
		}
		key := row.Proposed.GroupKey
		if _, ok := groups[key]; !ok {
			order = append(order, key)
		}
		groups[key] = append(groups[key], row)
	}
	for _, key := range order {
		rows := groups[key]
		n, isNew, err := c.applyGroup(ctx, rows)
		if err != nil {
			first := rows[0]
			var verr *domain.ValidationError
			if errors.As(err, &verr) {
				// Point the reviewer at the source row rather than at a
				// product document they never saw.
				return 0, 0, domain.Invalid("rows."+first.ID.String(), "sheet %q row %d (%s): %s",
					first.SheetName, first.RowIndex+1, first.Proposed.Name, verr.Error())
			}
			return 0, 0, fmt.Errorf("sheet %q row %d: %w", first.SheetName, first.RowIndex+1, err)
		}
		applied += n
		if isNew {
			created++
		}
	}
	return created, applied, nil
}

// applyGroup commits the rows of one product.
func (c *committer) applyGroup(ctx context.Context, rows []domain.ImportRow) (applied int, createdNew bool, err error) {
	var target *uuid.UUID
	var additions []domain.ImportRow
	for _, r := range rows {
		if r.TargetProductID != nil {
			target = r.TargetProductID
		}
		if r.Decision == domain.DecisionMerge {
			p := r.Proposed
			if err := c.im.products.UpdateVariantInTx(ctx, c.q, *r.TargetProductID, *p.TargetVariantID, p.Prices, p.StockStatus); err != nil {
				return 0, false, err
			}
			applied++
			continue
		}
		additions = append(additions, r)
	}
	if len(additions) == 0 {
		return applied, false, nil
	}

	variants := make([]service.VariantInput, len(additions))
	for i, r := range additions {
		if variants[i], err = c.variantInput(ctx, r.Proposed); err != nil {
			return 0, false, err
		}
	}
	productID := uuid.Nil
	if target != nil {
		// Extend the existing product with the new variants.
		view, err := c.im.products.GetInTx(ctx, c.q, *target)
		if err != nil {
			return 0, false, err
		}
		doc := service.ToInput(view)
		doc.Variants = append(doc.Variants, variants...)
		if err := c.im.products.UpdateInTx(ctx, c.q, *target, doc, ""); err != nil {
			return 0, false, err
		}
		productID = *target
	} else {
		p := additions[0].Proposed
		brandID, err := c.brandID(ctx, p.Brand)
		if err != nil {
			return 0, false, err
		}
		in := service.ProductInput{
			CategoryID:          p.CategoryID,
			BrandID:             brandID,
			Name:                p.Name,
			Status:              p.Status,
			RetailPriceIsPublic: c.opts.RetailPricesPublic,
			Attributes:          p.ProductAttributes,
			Variants:            variants,
		}
		if p.Summary != "" {
			in.Summary = &p.Summary
		}
		if productID, err = c.im.products.CreateInTx(ctx, c.q, in); err != nil {
			return 0, false, err
		}
		createdNew = true
	}
	for _, r := range additions {
		if err := c.q.SetImportRowTarget(ctx, gen.SetImportRowTargetParams{ID: r.ID, TargetProductID: &productID}); err != nil {
			return 0, false, err
		}
	}
	return applied + len(additions), createdNew, nil
}

func (c *committer) variantInput(ctx context.Context, p domain.ImportProposal) (service.VariantInput, error) {
	supplierID, err := c.supplierID(ctx, p.Supplier)
	if err != nil {
		return service.VariantInput{}, err
	}
	v := service.VariantInput{
		SKU:         p.SKU,
		SupplierID:  supplierID,
		StockStatus: p.StockStatus,
		Attributes:  p.VariantAttributes,
		Prices:      p.Prices,
	}
	if p.SupplierItemNo != "" {
		v.SupplierItemNo = &p.SupplierItemNo
	}
	if p.ModelNo != "" {
		v.ModelNo = &p.ModelNo
	}
	return v, nil
}
