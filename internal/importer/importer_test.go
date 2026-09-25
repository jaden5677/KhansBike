package importer

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/khansbikezone/bikezone-api/internal/domain"
)

func testBindings() []domain.CategoryAttribute {
	colour := domain.Attribute{ID: uuid.New(), Key: "colour", Label: "Colour", DataType: domain.DataTypeColor}
	colour.Options = []domain.AttributeOption{
		{ID: uuid.New(), Value: "black", Label: "Black"},
		{ID: uuid.New(), Value: "red", Label: "Red"},
	}
	wheel := domain.Attribute{ID: uuid.New(), Key: "wheel_size", Label: "Wheel Size", DataType: domain.DataTypeEnum}
	wheel.Options = []domain.AttributeOption{
		{ID: uuid.New(), Value: "20", Label: `20"`},
		{ID: uuid.New(), Value: "26", Label: `26"`},
	}
	width := domain.Attribute{ID: uuid.New(), Key: "width", Label: "Width", DataType: domain.DataTypeNumberRange}
	return []domain.CategoryAttribute{
		{Attribute: wheel, IsRequired: true},
		{Attribute: width},
		{Attribute: colour, IsVariantAxis: true},
	}
}

func testContext() *sheetContext {
	return &sheetContext{
		category:  &domain.Category{ID: uuid.New(), Name: "Tubes", Slug: "tubes"},
		bindings:  testBindings(),
		brands:    map[string]bool{"kenda": true},
		suppliers: map[string]bool{},
	}
}

func issueCodes(r *stagedRow) []string {
	var codes []string
	for _, is := range r.issues {
		codes = append(codes, is.Code)
	}
	return codes
}

func hasCode(r *stagedRow, code string) bool {
	for _, c := range issueCodes(r) {
		if c == code {
			return true
		}
	}
	return false
}

func TestParseStock(t *testing.T) {
	tests := map[string]domain.StockStatus{
		"12": domain.StockIn, "0": domain.StockOut, "In Stock": domain.StockIn, "LOW": domain.StockLow,
		"out of stock": domain.StockOut, "Special order": domain.StockSpecialOrder, "": domain.StockUnknown,
	}
	for in, want := range tests {
		if got, ok := parseStock(in); !ok || got != want {
			t.Errorf("parseStock(%q) = %s, %v; want %s", in, got, ok, want)
		}
	}
	if _, ok := parseStock("ask Khan"); ok {
		t.Error("unrecognised stock text accepted")
	}
}

func TestParseAttributeCell(t *testing.T) {
	b := testBindings()
	wheel, width, colour := &b[0].Attribute, &b[1].Attribute, &b[2].Attribute
	tests := []struct {
		attr *domain.Attribute
		cell string
		want string
	}{
		{wheel, `20"`, `"20"`},
		{wheel, "20 inch", `"20"`},
		{wheel, "26", `"26"`},
		{colour, " black ", `"black"`},
		{width, "1.95/2.125", `{"high":2.125,"low":1.95}`},
		{width, "1.95 - 2.125 in", `{"high":2.125,"low":1.95}`},
		{width, "2.125", `{"high":2.125,"low":2.125}`},
	}
	for _, tc := range tests {
		got, err := parseAttributeCell(tc.attr, tc.cell)
		if err != nil {
			t.Errorf("%s %q: %v", tc.attr.Key, tc.cell, err)
			continue
		}
		if string(got) != tc.want {
			t.Errorf("%s %q = %s, want %s", tc.attr.Key, tc.cell, got, tc.want)
		}
	}
	if _, err := parseAttributeCell(width, "2.125/1.95"); err == nil {
		t.Error("inverted range accepted")
	}
}

func TestMatchOptionIgnoringPunctuation(t *testing.T) {
	valve := &domain.Attribute{Key: "valve_type", Label: "Valve Type", DataType: domain.DataTypeEnum, Options: []domain.AttributeOption{
		{ID: uuid.New(), Value: "av", Label: "A/V (Schrader)"},
		{ID: uuid.New(), Value: "fv", Label: "F/V (Presta)"},
	}}
	for _, cell := range []string{"A/V", "a.v", "AV"} {
		if o, err := matchOption(valve, cell); err != nil || o.Value != "av" {
			t.Errorf("matchOption(%q) = %v, %v; want av", cell, o, err)
		}
	}
	// When punctuation-blind matching is ambiguous it must not guess.
	ambiguous := &domain.Attribute{Key: "code", Label: "Code", DataType: domain.DataTypeEnum, Options: []domain.AttributeOption{
		{ID: uuid.New(), Value: "ab-c", Label: "First"},
		{ID: uuid.New(), Value: "a-bc", Label: "Second"},
	}}
	if o, err := matchOption(ambiguous, "abc"); err == nil {
		t.Errorf("ambiguous cell matched %q", o.Value)
	}
}

func TestMatchOptionSuggestsTypos(t *testing.T) {
	b := testBindings()
	_, err := matchOption(&b[2].Attribute, "Blck")
	if err == nil || !strings.Contains(err.Error(), `did you mean "Black"`) {
		t.Errorf("typo error = %v, want a suggestion of Black", err)
	}
	_, err = matchOption(&b[2].Attribute, "Chartreuse")
	if err == nil || strings.Contains(err.Error(), "did you mean") {
		t.Errorf("unrelated value error = %v, want no suggestion", err)
	}
}

func TestLevenshtein(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want int
	}{{"", "", 0}, {"black", "black", 0}, {"blck", "black", 1}, {"kitten", "sitting", 3}} {
		if got := levenshtein(tc.a, tc.b); got != tc.want {
			t.Errorf("levenshtein(%q,%q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestParseSheet(t *testing.T) {
	rows := [][]string{
		{"Khan's Bike Zone price list"}, // a title row above the header
		{},
		{"Description", "Brand", "Colour", "Wheel Size", "Width", "Retail", "Cost", "Shelf"},
		{"Tube 20 x 1.95/2.125 A/V", "Kenda ", "Black", `20"`, "1.95/2.125", "35", "4.4000000000000004", "A3"},
		{"Tube 26 A/V", "Duro", "Blck", "26", "", "38", "", ""},
		{"", "", "", "", "", "", "", ""}, // blank rows are ignored
		{"", "Kenda", "", "", "", "12", "", ""},
	}
	got := parseSheet("Tubes", rows, testContext())
	if len(got) != 3 {
		t.Fatalf("staged %d rows, want 3", len(got))
	}

	first := got[0]
	p := first.proposal
	if first.index != 3 || p.Name != "Tube 20 x 1.95/2.125 A/V" || p.Brand != "Kenda" {
		t.Errorf("first row parsed as index %d %+v", first.index, p)
	}
	if p.Prices[domain.TierRetailTTD] != "35.00" || p.Prices[domain.TierCostUSD] != "4.40" {
		t.Errorf("prices = %v", p.Prices)
	}
	if string(p.ProductAttributes["wheel_size"]) != `"20"` || string(p.VariantAttributes["colour"]) != `"black"` {
		t.Errorf("attributes: product=%s variant=%s", p.ProductAttributes, p.VariantAttributes)
	}
	for _, code := range []string{"brand_whitespace", "price_rounded", "unmapped_columns", "generated_sku"} {
		if !hasCode(first, code) {
			t.Errorf("first row missing issue %q (has %v)", code, issueCodes(first))
		}
	}
	if first.has(domain.SeverityError) {
		t.Errorf("first row has errors: %v", first.issues)
	}

	second := got[1]
	if !hasCode(second, "unknown_option") || !hasCode(second, "new_brand") {
		t.Errorf("second row issues = %v, want unknown_option and new_brand", issueCodes(second))
	}

	third := got[2]
	if !hasCode(third, "missing_name") || !hasCode(third, "missing_required_attribute") {
		t.Errorf("third row issues = %v", issueCodes(third))
	}
}

func TestParseSheetWithoutHeader(t *testing.T) {
	got := parseSheet("Tubes", [][]string{{"just", "some"}, {"numbers", "1"}}, testContext())
	if len(got) != 1 || !hasCode(got[0], "no_header_row") || got[0].decision != domain.DecisionSkip {
		t.Errorf("headerless sheet staged as %+v", got)
	}
}

// The workbook's helmet sheet has six identical rows whose colour was never
// recorded: they must become one product, flagged for review.
func TestGroupRowsCollapsesIdenticalDuplicates(t *testing.T) {
	ctx := testContext()
	ctx.bindings = nil // helmets bind no attributes here
	header := []string{"Description", "Brand", "Item No", "Retail"}
	rows := [][]string{header}
	for i := 0; i < 6; i++ {
		rows = append(rows, []string{"Bicycle Helmet Kids / Multi Sport", "Kenda", "CAS009", "90"})
	}
	staged := parseSheet("Helmets", rows, ctx)
	groupRows(staged)

	kept := 0
	for _, r := range staged {
		if r.proposal.Status != domain.StatusNeedsReview {
			t.Errorf("row %d status %s, want needs_review", r.index, r.proposal.Status)
		}
		if r.decision == domain.DecisionAccept {
			kept++
			if !hasCode(r, "has_duplicates") {
				t.Errorf("kept row lacks has_duplicates: %v", issueCodes(r))
			}
		} else if !hasCode(r, "duplicate_row") {
			t.Errorf("skipped row lacks duplicate_row: %v", issueCodes(r))
		}
	}
	if kept != 1 {
		t.Errorf("kept %d rows, want exactly 1", kept)
	}
}

func TestGroupRowsMakesVariantsOfOneProduct(t *testing.T) {
	rows := [][]string{
		{"Description", "Brand", "Colour", "Wheel Size", "Retail"},
		{"Star Grips", "Kenda", "Black", "20", "45"},
		{"Star Grips", "Kenda", "Red", "20", "45"},
		{"Other Grips", "Kenda", "Red", "20", "45"},
	}
	staged := parseSheet("Grips", rows, testContext())
	groupRows(staged)
	if staged[0].proposal.GroupKey != staged[1].proposal.GroupKey {
		t.Error("colour variants of one product landed in different groups")
	}
	if staged[0].proposal.GroupKey == staged[2].proposal.GroupKey {
		t.Error("different products share a group")
	}
	if staged[0].proposal.SKU == staged[1].proposal.SKU {
		t.Errorf("variants share generated SKU %q", staged[0].proposal.SKU)
	}
	for _, r := range staged {
		if r.proposal.Status != domain.StatusActive || r.decision != domain.DecisionAccept {
			t.Errorf("clean row %d: status %s decision %s issues %v", r.index, r.proposal.Status, r.decision, issueCodes(r))
		}
	}
}

func TestGeneratedSKUIsDeterministic(t *testing.T) {
	p := &domain.ImportProposal{Brand: "Kenda", Name: "Star Grips",
		VariantAttributes: map[string]json.RawMessage{"colour": json.RawMessage(`"black"`)}}
	a, b := generatedSKU(p), generatedSKU(p)
	if a != b || a != "KENDA-STAR-GRIPS-BLACK" {
		t.Errorf("generatedSKU = %q then %q", a, b)
	}
}
