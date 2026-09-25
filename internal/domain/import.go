package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// ImportBatchStatus is the lifecycle of one uploaded workbook. A batch is
// staged as a dry run, may be reviewed, and then ends committed or aborted.
type ImportBatchStatus string

const (
	ImportDryRun    ImportBatchStatus = "dry_run"   // staged; nothing written to the catalogue yet
	ImportReviewing ImportBatchStatus = "reviewing" // a reviewer has changed at least one decision
	ImportCommitted ImportBatchStatus = "committed"
	ImportAborted   ImportBatchStatus = "aborted"
)

// IsOpen reports whether the batch can still be reviewed and committed.
func (s ImportBatchStatus) IsOpen() bool { return s == ImportDryRun || s == ImportReviewing }

// ImportDecision is what committing will do with one staged row.
type ImportDecision string

const (
	DecisionPending ImportDecision = "pending" // needs a human decision; blocks commit
	DecisionAccept  ImportDecision = "accept"  // create (or add a variant to the matched product)
	DecisionSkip    ImportDecision = "skip"    // ignore the row
	DecisionMerge   ImportDecision = "merge"   // update prices/stock of the matched existing variant
)

// Valid reports whether d is a known decision.
func (d ImportDecision) Valid() bool {
	switch d {
	case DecisionPending, DecisionAccept, DecisionSkip, DecisionMerge:
		return true
	default:
		return false
	}
}

// Issue severities. Errors block a row from being accepted; warnings import
// the product for review (status needs_review) rather than publishing it;
// info notes something the importer fixed or will do automatically.
const (
	SeverityError   = "error"
	SeverityWarning = "warning"
	SeverityInfo    = "info"
)

// ImportIssue is one defect detected in a staged row. Code is stable and
// machine-readable (e.g. "missing_retail_price") so a UI can group issues.
type ImportIssue struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Field    string `json:"field,omitempty"`
	Message  string `json:"message"`
}

// ImportProposal is the normalised reading of one source row: the product it
// belongs to and the variant it describes. Rows sharing a GroupKey become the
// variants of one product. Attribute values use the same JSON shapes as the
// admin product API, so committing goes through the ordinary product write
// path and its validation.
type ImportProposal struct {
	CategoryID        uuid.UUID                  `json:"categoryId"`
	GroupKey          string                     `json:"groupKey"`
	Name              string                     `json:"name"`
	Brand             string                     `json:"brand,omitempty"`
	Supplier          string                     `json:"supplier,omitempty"`
	Summary           string                     `json:"summary,omitempty"`
	Status            ProductStatus              `json:"status"`
	SKU               string                     `json:"sku"`
	SupplierItemNo    string                     `json:"supplierItemNo,omitempty"`
	ModelNo           string                     `json:"modelNo,omitempty"`
	StockStatus       StockStatus                `json:"stockStatus"`
	Prices            map[PriceTier]string       `json:"prices,omitempty"`
	ProductAttributes map[string]json.RawMessage `json:"productAttributes,omitempty"`
	VariantAttributes map[string]json.RawMessage `json:"variantAttributes,omitempty"`
	// TargetVariantID is the existing variant this row matched by SKU or
	// supplier item number; set together with the row's TargetProductID.
	TargetVariantID *uuid.UUID `json:"targetVariantId,omitempty"`
}

// ImportBatch is one uploaded workbook and a count of its rows per decision.
type ImportBatch struct {
	ID          uuid.UUID
	Filename    string
	SHA256      string // hex
	Status      ImportBatchStatus
	CreatedBy   *uuid.UUID
	CreatedAt   time.Time
	CommittedAt *time.Time
	Counts      map[ImportDecision]int
}

// ImportRow is one staged source row with its proposal, issues and decision.
type ImportRow struct {
	ID              uuid.UUID
	BatchID         uuid.UUID
	SheetName       string
	RowIndex        int               // 0-based row within the sheet
	Raw             map[string]string // header -> cell text, verbatim
	Proposed        ImportProposal
	Issues          []ImportIssue
	Decision        ImportDecision
	TargetProductID *uuid.UUID
}

// HasErrors reports whether any issue blocks the row from being accepted.
func (r *ImportRow) HasErrors() bool {
	for _, is := range r.Issues {
		if is.Severity == SeverityError {
			return true
		}
	}
	return false
}
