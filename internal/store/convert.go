package store

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/khansbikezone/bikezone-api/internal/domain"
	"github.com/khansbikezone/bikezone-api/internal/platform"
	"github.com/khansbikezone/bikezone-api/internal/store/gen"
)

// This file converts between sqlc's database types and domain types. sqlc
// emits a distinct row struct per query even when the columns are identical
// (GetProductByIDRow, GetProductBySlugRow, ...); Go allows converting between
// struct types with identical fields, so callers convert those to the one
// canonical row type each converter accepts, e.g.
// store.Product(gen.GetProductByIDRow(row)).

// ---- pgtype helpers ---------------------------------------------------------

// Text converts an optional string to a nullable text parameter.
func Text(s *string) pgtype.Text {
	if s == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *s, Valid: true}
}

// TextOrNull stores the empty string as NULL, for optional free-text fields.
func TextOrNull(s string) pgtype.Text {
	return pgtype.Text{String: s, Valid: s != ""}
}

// StringPtr converts a nullable text column to an optional string.
func StringPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	s := t.String
	return &s
}

// Timestamptz converts a time to a non-null timestamptz parameter.
func Timestamptz(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

// NullTimestamptz converts an optional time to a nullable timestamptz.
func NullTimestamptz(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return Timestamptz(*t)
}

// TimePtr converts a nullable timestamptz column to an optional time.
func TimePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

// Date converts a calendar date (time of day ignored) to a date parameter.
func Date(t time.Time) pgtype.Date {
	return pgtype.Date{Time: time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC), Valid: true}
}

// Float8 converts an optional float to a nullable double precision value.
func Float8(f *float64) pgtype.Float8 {
	if f == nil {
		return pgtype.Float8{}
	}
	return pgtype.Float8{Float64: *f, Valid: true}
}

func float64Ptr(f pgtype.Float8) *float64 {
	if !f.Valid {
		return nil
	}
	v := f.Float64
	return &v
}

// Bool converts an optional bool to a nullable boolean value.
func Bool(b *bool) pgtype.Bool {
	if b == nil {
		return pgtype.Bool{}
	}
	return pgtype.Bool{Bool: *b, Valid: true}
}

func boolPtr(b pgtype.Bool) *bool {
	if !b.Valid {
		return nil
	}
	v := b.Bool
	return &v
}

// Int4 converts an int to a non-null integer parameter.
func Int4(i int) pgtype.Int4 {
	return pgtype.Int4{Int32: int32(i), Valid: true}
}

func intOrZero(i pgtype.Int4) int {
	if !i.Valid {
		return 0
	}
	return int(i.Int32)
}

// ---- catalogue ----------------------------------------------------------------

// Category converts a category row. ProductCount and Children stay zero.
func Category(r gen.GetCategoryByIDRow) domain.Category {
	return domain.Category{
		ID:          r.ID,
		ParentID:    r.ParentID,
		Name:        r.Name,
		Slug:        r.Slug,
		Path:        r.Path,
		Position:    int(r.Position),
		Description: StringPtr(r.Description),
		HeroAssetID: r.HeroAssetID,
		IsActive:    r.IsActive,
		CreatedAt:   r.CreatedAt.Time,
		UpdatedAt:   r.UpdatedAt.Time,
	}
}

// Brand converts a brand row.
func Brand(r gen.Brand) domain.Brand {
	return domain.Brand{ID: r.ID, Name: r.Name, Slug: r.Slug, LogoAssetID: r.LogoAssetID, Position: int(r.Position)}
}

// Supplier converts a supplier row. ADMIN ONLY data.
func Supplier(r gen.Supplier) domain.Supplier {
	return domain.Supplier{ID: r.ID, Name: r.Name, Code: StringPtr(r.Code), Notes: StringPtr(r.Notes)}
}

// Attribute converts an attribute row; Options are loaded separately.
func Attribute(r gen.Attribute) domain.Attribute {
	return domain.Attribute{
		ID:           r.ID,
		Key:          r.Key,
		Label:        r.Label,
		DataType:     domain.DataType(r.DataType),
		Unit:         StringPtr(r.Unit),
		InputType:    r.InputType,
		IsFilterable: r.IsFilterable,
		IsSearchable: r.IsSearchable,
		HelpText:     StringPtr(r.HelpText),
		CreatedAt:    r.CreatedAt.Time,
		UpdatedAt:    r.UpdatedAt.Time,
	}
}

// AttributeOption converts an option row.
func AttributeOption(r gen.AttributeOption) domain.AttributeOption {
	return domain.AttributeOption{
		ID:          r.ID,
		AttributeID: r.AttributeID,
		Value:       r.Value,
		Label:       r.Label,
		SwatchHex:   StringPtr(r.SwatchHex),
		Position:    int(r.Position),
	}
}

// CategoryAttribute converts a binding joined with its attribute.
func CategoryAttribute(r gen.ListCategoryAttributesRow) domain.CategoryAttribute {
	return domain.CategoryAttribute{
		CategoryID: r.CategoryID,
		Attribute: domain.Attribute{
			ID:           r.AttributeID,
			Key:          r.Key,
			Label:        r.Label,
			DataType:     domain.DataType(r.DataType),
			Unit:         StringPtr(r.Unit),
			InputType:    r.InputType,
			IsFilterable: r.IsFilterable,
			IsSearchable: r.IsSearchable,
			HelpText:     StringPtr(r.HelpText),
			CreatedAt:    r.CreatedAt.Time,
			UpdatedAt:    r.UpdatedAt.Time,
		},
		Position:      int(r.Position),
		IsRequired:    r.IsRequired,
		IsVariantAxis: r.IsVariantAxis,
		LabelOverride: StringPtr(r.LabelOverride),
	}
}

// Product converts a product row. The attrs projection is deliberately not
// decoded: it exists for the filter path, and reads use the EAV rows.
func Product(r gen.GetProductByIDRow) domain.Product {
	return domain.Product{
		ID:                  r.ID,
		CategoryID:          r.CategoryID,
		BrandID:             r.BrandID,
		Name:                r.Name,
		Slug:                r.Slug,
		Summary:             StringPtr(r.Summary),
		Description:         StringPtr(r.Description),
		Status:              domain.ProductStatus(r.Status),
		IsFeatured:          r.IsFeatured,
		RetailPriceIsPublic: r.RetailPriceIsPublic,
		CreatedAt:           r.CreatedAt.Time,
		UpdatedAt:           r.UpdatedAt.Time,
		PublishedAt:         TimePtr(r.PublishedAt),
	}
}

// Variant converts a variant row. Attributes and Prices are loaded separately.
func Variant(r gen.ProductVariant) domain.Variant {
	return domain.Variant{
		ID:             r.ID,
		ProductID:      r.ProductID,
		SKU:            r.Sku,
		SupplierID:     r.SupplierID,
		SupplierItemNo: StringPtr(r.SupplierItemNo),
		ModelNo:        StringPtr(r.ModelNo),
		NameSuffix:     StringPtr(r.NameSuffix),
		Position:       int(r.Position),
		StockStatus:    domain.StockStatus(r.StockStatus),
		IsDefault:      r.IsDefault,
		CreatedAt:      r.CreatedAt.Time,
		UpdatedAt:      r.UpdatedAt.Time,
	}
}

// AttributeValue converts one EAV row. A multi_enum value spans several rows
// (one per option); callers merge them by attribute and variant.
func AttributeValue(r gen.ProductAttributeValue) domain.AttributeValue {
	v := domain.AttributeValue{
		AttributeID: r.AttributeID,
		VariantID:   r.VariantID,
		Text:        StringPtr(r.ValueText),
		Num:         float64Ptr(r.ValueNum),
		NumLow:      float64Ptr(r.ValueNumLow),
		NumHigh:     float64Ptr(r.ValueNumHigh),
		Bool:        boolPtr(r.ValueBool),
		ETRTO:       StringPtr(r.Etrto),
	}
	if r.OptionID != nil {
		v.OptionIDs = []uuid.UUID{*r.OptionID}
	}
	return v
}

// Price converts a price row into exact Money.
func Price(r gen.Price) domain.Price {
	return domain.Price{
		ID:            r.ID,
		VariantID:     r.VariantID,
		Tier:          domain.PriceTier(r.Tier),
		Amount:        platform.NewMoney(r.AmountMinor, r.Currency),
		EffectiveFrom: r.EffectiveFrom.Time,
		SourceNote:    StringPtr(r.SourceNote),
		CreatedAt:     r.CreatedAt.Time,
	}
}

// ---- media ----------------------------------------------------------------------

// MediaAsset converts an asset row; Renditions are loaded separately.
func MediaAsset(r gen.MediaAsset) domain.MediaAsset {
	a := domain.MediaAsset{
		ID:               r.ID,
		SHA256:           hex.EncodeToString(r.Sha256),
		OriginalFilename: r.OriginalFilename,
		MIME:             r.Mime,
		ByteSize:         r.ByteSize,
		Width:            intOrZero(r.Width),
		Height:           intOrZero(r.Height),
		Blurhash:         r.Blurhash.String,
		DominantHex:      r.DominantHex.String,
		StorageKey:       r.StorageKey,
		Status:           domain.AssetStatus(r.Status),
		FailureReason:    StringPtr(r.FailureReason),
		CreatedAt:        r.CreatedAt.Time,
	}
	if r.UploadedBy != nil {
		a.UploadedBy = *r.UploadedBy
	}
	return a
}

// Rendition converts a rendition row.
func Rendition(r gen.MediaRendition) domain.Rendition {
	return domain.Rendition{
		ID:         r.ID,
		AssetID:    r.AssetID,
		Width:      int(r.Width),
		Height:     int(r.Height),
		Format:     r.Format,
		StorageKey: r.StorageKey,
		ByteSize:   r.ByteSize,
	}
}

// ProductMedia converts a product/asset association; Asset is loaded separately.
func ProductMedia(r gen.ProductMedium) domain.ProductMedia {
	return domain.ProductMedia{
		ID:        r.ID,
		ProductID: r.ProductID,
		VariantID: r.VariantID,
		AssetID:   r.AssetID,
		Role:      domain.MediaRole(r.Role),
		Position:  int(r.Position),
		AltText:   StringPtr(r.AltText),
	}
}

// ---- audit and import ------------------------------------------------------------

// AuditEntry converts an audit log row.
func AuditEntry(r gen.ListAuditLogRow) domain.AuditEntry {
	return domain.AuditEntry{
		ID:          r.ID,
		ActorUserID: r.ActorUserID,
		ActorEmail:  StringPtr(r.ActorEmail),
		ActorKind:   domain.ActorKind(r.ActorKind),
		Action:      r.Action,
		EntityType:  r.EntityType,
		EntityID:    r.EntityID,
		Before:      r.Before,
		After:       r.After,
		IP:          r.Ip,
		CreatedAt:   r.CreatedAt.Time,
	}
}

// ImportBatch converts a batch row; Counts are loaded separately.
func ImportBatch(r gen.ImportBatch) domain.ImportBatch {
	return domain.ImportBatch{
		ID:          r.ID,
		Filename:    r.Filename,
		SHA256:      hex.EncodeToString(r.Sha256),
		Status:      domain.ImportBatchStatus(r.Status),
		CreatedBy:   r.CreatedBy,
		CreatedAt:   r.CreatedAt.Time,
		CommittedAt: TimePtr(r.CommittedAt),
	}
}

// ImportRow converts a staged row, decoding its JSON columns.
func ImportRow(r gen.ImportRow) (domain.ImportRow, error) {
	row := domain.ImportRow{
		ID:              r.ID,
		BatchID:         r.BatchID,
		SheetName:       r.SheetName,
		RowIndex:        int(r.RowIndex),
		Decision:        domain.ImportDecision(r.Decision),
		TargetProductID: r.TargetProductID,
	}
	if err := json.Unmarshal(r.Raw, &row.Raw); err != nil {
		return row, fmt.Errorf("decode import row %s raw: %w", r.ID, err)
	}
	if err := json.Unmarshal(r.Proposed, &row.Proposed); err != nil {
		return row, fmt.Errorf("decode import row %s proposal: %w", r.ID, err)
	}
	if err := json.Unmarshal(r.Issues, &row.Issues); err != nil {
		return row, fmt.Errorf("decode import row %s issues: %w", r.ID, err)
	}
	return row, nil
}
