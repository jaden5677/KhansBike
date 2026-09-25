//go:build integration

package importer_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/xuri/excelize/v2"

	"github.com/khansbikezone/bikezone-api/internal/app"
	"github.com/khansbikezone/bikezone-api/internal/domain"
	"github.com/khansbikezone/bikezone-api/internal/importer"
	"github.com/khansbikezone/bikezone-api/internal/service"
	"github.com/khansbikezone/bikezone-api/internal/testutil"
)

// setup creates a Tubes category with a required wheel size and a valve type
// that varies by variant.
func setup(t *testing.T) (*app.App, context.Context) {
	t.Helper()
	ctx := context.Background()
	a := testutil.App(t, nil)
	wheel, err := a.Taxonomy.CreateAttribute(ctx, service.AttributeInput{Key: "wheel_size", Label: "Wheel Size", DataType: domain.DataTypeEnum,
		Options: []service.OptionInput{{Value: "20", Label: `20"`}, {Value: "26", Label: `26"`}}})
	if err != nil {
		t.Fatal(err)
	}
	valve, err := a.Taxonomy.CreateAttribute(ctx, service.AttributeInput{Key: "valve_type", Label: "Valve Type", DataType: domain.DataTypeEnum,
		Options: []service.OptionInput{{Value: "av", Label: "A/V (Schrader)"}, {Value: "fv", Label: "F/V (Presta)"}}})
	if err != nil {
		t.Fatal(err)
	}
	tubes, err := a.Taxonomy.CreateCategory(ctx, service.CategoryInput{Name: "Tubes"})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Taxonomy.BindAttribute(ctx, tubes.ID, wheel.ID, service.BindingInput{IsRequired: true}); err != nil {
		t.Fatal(err)
	}
	if err := a.Taxonomy.BindAttribute(ctx, tubes.ID, valve.ID, service.BindingInput{Position: 1, IsVariantAxis: true}); err != nil {
		t.Fatal(err)
	}
	return a, ctx
}

// sum adds at run time. (0.1 + 0.2 written as constants is exactly 0.3:
// Go evaluates constant expressions exactly. The float noise spreadsheets
// store only appears in run-time float64 arithmetic.)
func sum(a, b float64) float64 { return a + b }

// workbook builds an .xlsx with a Tubes sheet (plus any extra sheets).
func workbook(t *testing.T, retail20 float64, extra ...string) []byte {
	t.Helper()
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	if err := f.SetSheetName("Sheet1", "Tubes"); err != nil {
		t.Fatal(err)
	}
	rows := [][]any{
		{"Tubes price list"},
		{"Description", "Brand", "Item No", "Wheel Size", "Valve Type", "Cost", "Retail", "Qty"},
		{"Tube 20 x 1.95", "Kenda ", "K20", `20"`, "A/V", sum(0.1, 0.2), retail20, 12}, // stored as 0.30000000000000004
		{"Tube 20 x 1.95", "Kenda ", "K20F", "20", "F/V (Presta)", 0.35, retail20, 0},
		{"Tube 26 x 2.1", "Duro", "D26", "26", "Schraeder", 0.5, 38, 4},
	}
	for i, r := range rows {
		cell, _ := excelize.CoordinatesToCellName(1, i+1)
		if err := f.SetSheetRow("Tubes", cell, &r); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range extra {
		if _, err := f.NewSheet(name); err != nil {
			t.Fatal(err)
		}
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func issueCodes(rows []domain.ImportRow) map[string]int {
	out := map[string]int{}
	for _, r := range rows {
		for _, is := range r.Issues {
			out[is.Code]++
		}
	}
	return out
}

func TestStageReviewCommitAndReimport(t *testing.T) {
	t.Parallel()
	a, ctx := setup(t)
	im := a.Importer

	b, err := im.Stage(ctx, "prices.xlsx", bytes.NewReader(workbook(t, 35, "Unknown Things")))
	if err != nil {
		t.Fatalf("Stage: %v", err)
	}
	if b.Status != domain.ImportDryRun || b.Counts[domain.DecisionAccept] != 2 || b.Counts[domain.DecisionPending] != 1 || b.Counts[domain.DecisionSkip] != 1 {
		t.Fatalf("staged batch = %+v", b)
	}
	rows, _, err := im.Rows(ctx, b.ID, importer.RowQuery{})
	if err != nil {
		t.Fatal(err)
	}
	codes := issueCodes(rows)
	for _, want := range []string{"brand_whitespace", "price_rounded", "unknown_option", "unknown_sheet", "new_brand"} {
		if codes[want] == 0 {
			t.Errorf("expected issue %q among %v", want, codes)
		}
	}
	if rows[0].Proposed.Prices[domain.TierCostUSD] != "0.30" {
		t.Errorf("0.1+0.2 read as %q, want 0.30", rows[0].Proposed.Prices[domain.TierCostUSD])
	}
	if rows[0].Proposed.GroupKey != rows[1].Proposed.GroupKey {
		t.Error("the two 20-inch valve variants were not grouped into one product")
	}

	// Pending rows block the commit; accepting an errored row is refused.
	if _, err := im.Commit(ctx, b.ID, importer.CommitOptions{}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("commit with pending rows: err = %v", err)
	}
	pending, _, _ := im.Rows(ctx, b.ID, importer.RowQuery{Decision: domain.DecisionPending})
	if _, err := im.SetDecision(ctx, b.ID, pending[0].ID, domain.DecisionAccept); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("accepting an errored row: err = %v", err)
	}
	if _, err := im.SetDecision(ctx, b.ID, pending[0].ID, domain.DecisionSkip); err != nil {
		t.Fatal(err)
	}

	committed, err := im.Commit(ctx, b.ID, importer.CommitOptions{RetailPricesPublic: true})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if committed.Status != domain.ImportCommitted {
		t.Errorf("status after commit = %s", committed.Status)
	}
	if _, err := im.Commit(ctx, b.ID, importer.CommitOptions{}); !errors.Is(err, domain.ErrConflict) {
		t.Errorf("second commit: err = %v, want conflict", err)
	}

	// One product with two valve variants, under the existing-or-new brand,
	// published with its retail price.
	page, err := a.Catalog.ListProducts(ctx, domain.ProductQuery{CategorySlug: "tubes"})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || page.Items[0].VariantCount != 2 || page.Items[0].Product.Brand.Name != "Kenda" ||
		page.Items[0].FromPrice == nil || page.Items[0].FromPrice.Decimal() != "35.00" {
		t.Fatalf("imported catalogue = %+v", page.Items)
	}

	// Re-importing the same sheet with a new price merges instead of duplicating.
	again, err := im.Stage(ctx, "prices-v2.xlsx", bytes.NewReader(workbook(t, 36.5)))
	if err != nil {
		t.Fatal(err)
	}
	if again.Counts[domain.DecisionMerge] != 2 || again.Counts[domain.DecisionAccept] != 0 {
		t.Fatalf("re-import counts = %v, want 2 merges", again.Counts)
	}
	pending, _, _ = im.Rows(ctx, again.ID, importer.RowQuery{Decision: domain.DecisionPending})
	for _, r := range pending {
		if _, err := im.SetDecision(ctx, again.ID, r.ID, domain.DecisionSkip); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := im.Commit(ctx, again.ID, importer.CommitOptions{}); err != nil {
		t.Fatalf("commit re-import: %v", err)
	}
	page, _ = a.Catalog.ListProducts(ctx, domain.ProductQuery{CategorySlug: "tubes"})
	if page.Total != 1 || page.Items[0].FromPrice.Decimal() != "36.50" {
		t.Errorf("after re-import: total %d, from %v", page.Total, page.Items[0].FromPrice)
	}
}

func TestAbort(t *testing.T) {
	t.Parallel()
	a, ctx := setup(t)
	b, err := a.Importer.Stage(ctx, "prices.xlsx", bytes.NewReader(workbook(t, 35)))
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Importer.Abort(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Importer.Commit(ctx, b.ID, importer.CommitOptions{}); !errors.Is(err, domain.ErrConflict) {
		t.Errorf("commit after abort: err = %v, want conflict", err)
	}
	if _, err := a.Importer.Stage(ctx, "junk.xlsx", bytes.NewReader([]byte("not a workbook"))); !errors.Is(err, domain.ErrValidation) {
		t.Errorf("staging junk: err = %v, want validation", err)
	}
}
