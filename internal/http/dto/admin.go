package dto

import (
	"encoding/json"
	"net/netip"
	"time"

	"github.com/google/uuid"

	"github.com/khansbikezone/bikezone-api/internal/auth"
	"github.com/khansbikezone/bikezone-api/internal/domain"
	"github.com/khansbikezone/bikezone-api/internal/service"
)

// ---- identity -----------------------------------------------------------------

// User is the signed-in user.
type User struct {
	ID          uuid.UUID   `json:"id"`
	Email       string      `json:"email"`
	DisplayName string      `json:"displayName"`
	Role        domain.Role `json:"role"`
}

// Session describes the caller's authentication. CSRFToken is present for
// browser sessions and must be sent as X-CSRF-Token on every write.
type Session struct {
	User      User             `json:"user"`
	Kind      domain.ActorKind `json:"kind"` // "admin" (browser) or "device" (paired phone)
	CSRFToken string           `json:"csrfToken,omitempty"`
	ExpiresAt *time.Time       `json:"expiresAt,omitempty"`
}

// NewSession maps a principal.
func NewSession(p *auth.Principal, csrf string, expires *time.Time) Session {
	return Session{
		User:      User{ID: p.UserID, Email: p.Email, DisplayName: p.DisplayName, Role: p.Role},
		Kind:      p.Kind,
		CSRFToken: csrf,
		ExpiresAt: expires,
	}
}

// PairingCode is shown to the owner as a QR code; URL is what the QR encodes.
type PairingCode struct {
	Code      string    `json:"code"`
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// Device is a paired phone. Its token is never shown again after pairing.
type Device struct {
	ID         uuid.UUID  `json:"id"`
	Name       string     `json:"name"`
	LastSeenAt *time.Time `json:"lastSeenAt,omitempty"`
	RevokedAt  *time.Time `json:"revokedAt,omitempty"`
	CreatedAt  time.Time  `json:"createdAt"`
}

// NewDevice maps a device.
func NewDevice(d domain.DeviceToken) Device {
	return Device{ID: d.ID, Name: d.Name, LastSeenAt: d.LastSeenAt, RevokedAt: d.RevokedAt, CreatedAt: d.CreatedAt}
}

// PairedDevice is the one-time result of pairing: the bearer token to store
// on the phone.
type PairedDevice struct {
	Token  string `json:"token"`
	Device Device `json:"device"`
}

// ---- taxonomy -------------------------------------------------------------------

// Category is the admin view of a category.
type Category struct {
	ID          uuid.UUID  `json:"id"`
	ParentID    *uuid.UUID `json:"parentId"`
	Name        string     `json:"name"`
	Slug        string     `json:"slug"`
	Path        string     `json:"path"`
	Position    int        `json:"position"`
	Description *string    `json:"description"`
	HeroAssetID *uuid.UUID `json:"heroAssetId"`
	IsActive    bool       `json:"isActive"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
}

// NewCategory maps a category.
func NewCategory(c domain.Category) Category {
	return Category{
		ID: c.ID, ParentID: c.ParentID, Name: c.Name, Slug: c.Slug, Path: c.Path, Position: c.Position,
		Description: c.Description, HeroAssetID: c.HeroAssetID, IsActive: c.IsActive,
		CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
	}
}

// Option is an attribute option.
type Option struct {
	ID        uuid.UUID `json:"id"`
	Value     string    `json:"value"`
	Label     string    `json:"label"`
	SwatchHex *string   `json:"swatchHex"`
	Position  int       `json:"position"`
}

// NewOption maps an option.
func NewOption(o domain.AttributeOption) Option {
	return Option{ID: o.ID, Value: o.Value, Label: o.Label, SwatchHex: o.SwatchHex, Position: o.Position}
}

// Attribute is a registered attribute with its options.
type Attribute struct {
	ID           uuid.UUID       `json:"id"`
	Key          string          `json:"key"`
	Label        string          `json:"label"`
	DataType     domain.DataType `json:"dataType"`
	Unit         *string         `json:"unit"`
	InputType    string          `json:"inputType"`
	IsFilterable bool            `json:"isFilterable"`
	IsSearchable bool            `json:"isSearchable"`
	HelpText     *string         `json:"helpText"`
	Options      []Option        `json:"options"`
	CreatedAt    time.Time       `json:"createdAt"`
	UpdatedAt    time.Time       `json:"updatedAt"`
}

// NewAttribute maps an attribute.
func NewAttribute(a domain.Attribute) Attribute {
	out := Attribute{
		ID: a.ID, Key: a.Key, Label: a.Label, DataType: a.DataType, Unit: a.Unit, InputType: a.InputType,
		IsFilterable: a.IsFilterable, IsSearchable: a.IsSearchable, HelpText: a.HelpText,
		Options: make([]Option, len(a.Options)), CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt,
	}
	for i, o := range a.Options {
		out.Options[i] = NewOption(o)
	}
	return out
}

// Binding is an attribute as it applies within one category.
type Binding struct {
	Attribute      Attribute `json:"attribute"`
	Position       int       `json:"position"`
	IsRequired     bool      `json:"isRequired"`
	IsVariantAxis  bool      `json:"isVariantAxis"`
	LabelOverride  *string   `json:"labelOverride"`
	EffectiveLabel string    `json:"effectiveLabel"`
}

// NewBindings maps a category's bindings.
func NewBindings(bs []domain.CategoryAttribute) []Binding {
	out := make([]Binding, len(bs))
	for i := range bs {
		b := &bs[i]
		out[i] = Binding{
			Attribute: NewAttribute(b.Attribute), Position: b.Position, IsRequired: b.IsRequired,
			IsVariantAxis: b.IsVariantAxis, LabelOverride: b.LabelOverride, EffectiveLabel: b.EffectiveLabel(),
		}
	}
	return out
}

// Brand is the admin view of a brand.
type Brand struct {
	ID          uuid.UUID  `json:"id"`
	Name        string     `json:"name"`
	Slug        string     `json:"slug"`
	LogoAssetID *uuid.UUID `json:"logoAssetId"`
	Position    int        `json:"position"`
}

// NewBrand maps a brand.
func NewBrand(b domain.Brand) Brand {
	return Brand{ID: b.ID, Name: b.Name, Slug: b.Slug, LogoAssetID: b.LogoAssetID, Position: b.Position}
}

// Supplier is ADMIN ONLY.
type Supplier struct {
	ID    uuid.UUID `json:"id"`
	Name  string    `json:"name"`
	Code  *string   `json:"code"`
	Notes *string   `json:"notes"`
}

// NewSupplier maps a supplier.
func NewSupplier(s domain.Supplier) Supplier {
	return Supplier{ID: s.ID, Name: s.Name, Code: s.Code, Notes: s.Notes}
}

// ---- products -------------------------------------------------------------------

// AdminProductSummary is a product in the admin list.
type AdminProductSummary struct {
	ProductCard
	Status    domain.ProductStatus `json:"status"`
	UpdatedAt time.Time            `json:"updatedAt"`
}

// NewAdminProductPage maps an admin listing page.
func NewAdminProductPage(pg domain.ProductPage, urls URLFunc) Page[AdminProductSummary] {
	out := Page[AdminProductSummary]{Items: make([]AdminProductSummary, len(pg.Items)), Total: &pg.Total, NextCursor: pg.NextCursor}
	for i, s := range pg.Items {
		out.Items[i] = AdminProductSummary{ProductCard: NewProductCard(s, urls), Status: s.Product.Status, UpdatedAt: s.Product.UpdatedAt}
	}
	return out
}

// AdminPrice is a variant's current price in one tier.
type AdminPrice struct {
	Amount        string `json:"amount"`
	Currency      string `json:"currency"`
	EffectiveFrom string `json:"effectiveFrom"` // YYYY-MM-DD
}

// AdminVariant carries everything, supplier data and every price tier
// included. Attributes use the same value shapes the write API accepts, so a
// client can edit and send them back unchanged.
type AdminVariant struct {
	ID             uuid.UUID                       `json:"id"`
	SKU            string                          `json:"sku"`
	SupplierID     *uuid.UUID                      `json:"supplierId"`
	SupplierItemNo *string                         `json:"supplierItemNo"`
	ModelNo        *string                         `json:"modelNo"`
	NameSuffix     *string                         `json:"nameSuffix"`
	Position       int                             `json:"position"`
	StockStatus    domain.StockStatus              `json:"stockStatus"`
	IsDefault      bool                            `json:"isDefault"`
	Attributes     map[string]any                  `json:"attributes"`
	Prices         map[domain.PriceTier]AdminPrice `json:"prices"`
}

// AdminMedia is an image attachment with its processing state.
type AdminMedia struct {
	ID        uuid.UUID          `json:"id"`
	AssetID   uuid.UUID          `json:"assetId"`
	VariantID *uuid.UUID         `json:"variantId"`
	Role      domain.MediaRole   `json:"role"`
	Position  int                `json:"position"`
	AltText   *string            `json:"altText"`
	Status    domain.AssetStatus `json:"status"`
	Image     *Image             `json:"image"`
}

// NewAdminMedia maps an attachment.
func NewAdminMedia(m domain.ProductMedia, urls URLFunc) AdminMedia {
	out := AdminMedia{ID: m.ID, AssetID: m.AssetID, VariantID: m.VariantID, Role: m.Role, Position: m.Position, AltText: m.AltText}
	if m.Asset != nil {
		out.Status = m.Asset.Status
		out.Image = NewImage(m.Asset, m.AltText, urls)
	}
	return out
}

// AdminProduct is the full product document as the admin clients edit it.
type AdminProduct struct {
	ID                  uuid.UUID            `json:"id"`
	CategoryID          uuid.UUID            `json:"categoryId"`
	Category            CategoryRef          `json:"category"`
	BrandID             *uuid.UUID           `json:"brandId"`
	Brand               *BrandRef            `json:"brand"`
	Name                string               `json:"name"`
	Slug                string               `json:"slug"`
	Summary             *string              `json:"summary"`
	Description         *string              `json:"description"`
	Status              domain.ProductStatus `json:"status"`
	IsFeatured          bool                 `json:"isFeatured"`
	RetailPriceIsPublic bool                 `json:"retailPriceIsPublic"`
	Attributes          map[string]any       `json:"attributes"`
	Variants            []AdminVariant       `json:"variants"`
	Media               []AdminMedia         `json:"media"`
	CreatedAt           time.Time            `json:"createdAt"`
	UpdatedAt           time.Time            `json:"updatedAt"`
	PublishedAt         *time.Time           `json:"publishedAt"`
}

func valueMap(views []service.AttributeView) map[string]any {
	m := make(map[string]any, len(views))
	for _, v := range views {
		m[v.Key] = v.Value
	}
	return m
}

// NewAdminProduct maps an admin product view.
func NewAdminProduct(v *service.ProductView, urls URLFunc) AdminProduct {
	p := v.Product
	out := AdminProduct{
		ID: p.ID, CategoryID: p.CategoryID, Category: categoryRef(p.Category), BrandID: p.BrandID, Brand: brandRef(p.Brand),
		Name: p.Name, Slug: p.Slug, Summary: p.Summary, Description: p.Description, Status: p.Status,
		IsFeatured: p.IsFeatured, RetailPriceIsPublic: p.RetailPriceIsPublic, Attributes: valueMap(v.Attributes),
		Variants: make([]AdminVariant, len(p.Variants)), Media: make([]AdminMedia, len(p.Media)),
		CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt, PublishedAt: p.PublishedAt,
	}
	for i, vr := range p.Variants {
		av := AdminVariant{
			ID: vr.ID, SKU: vr.SKU, SupplierID: vr.SupplierID, SupplierItemNo: vr.SupplierItemNo, ModelNo: vr.ModelNo,
			NameSuffix: vr.NameSuffix, Position: vr.Position, StockStatus: vr.StockStatus, IsDefault: vr.IsDefault,
			Attributes: valueMap(v.VariantAttributes[vr.ID]), Prices: map[domain.PriceTier]AdminPrice{},
		}
		for _, pr := range vr.Prices {
			av.Prices[pr.Tier] = AdminPrice{Amount: pr.Amount.Decimal(), Currency: pr.Amount.Currency(), EffectiveFrom: pr.EffectiveFrom.Format(time.DateOnly)}
		}
		out.Variants[i] = av
	}
	for i, m := range p.Media {
		out.Media[i] = NewAdminMedia(m, urls)
	}
	return out
}

// ---- media ------------------------------------------------------------------------

// Rendition is one derived image file.
type Rendition struct {
	URL      string `json:"url"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	Format   string `json:"format"`
	ByteSize int64  `json:"byteSize"`
}

// Asset is an upload and its processing state.
type Asset struct {
	ID               uuid.UUID          `json:"id"`
	SHA256           string             `json:"sha256"`
	OriginalFilename string             `json:"originalFilename"`
	MIME             string             `json:"mime"`
	ByteSize         int64              `json:"byteSize"`
	Width            int                `json:"width"`
	Height           int                `json:"height"`
	Blurhash         string             `json:"blurhash,omitempty"`
	DominantHex      string             `json:"dominantHex,omitempty"`
	Status           domain.AssetStatus `json:"status"`
	FailureReason    *string            `json:"failureReason,omitempty"`
	Renditions       []Rendition        `json:"renditions"`
	CreatedAt        time.Time          `json:"createdAt"`
}

// NewAsset maps an asset.
func NewAsset(a domain.MediaAsset, urls URLFunc) Asset {
	out := Asset{
		ID: a.ID, SHA256: a.SHA256, OriginalFilename: a.OriginalFilename, MIME: a.MIME, ByteSize: a.ByteSize,
		Width: a.Width, Height: a.Height, Blurhash: a.Blurhash, DominantHex: a.DominantHex, Status: a.Status,
		FailureReason: a.FailureReason, Renditions: make([]Rendition, len(a.Renditions)), CreatedAt: a.CreatedAt,
	}
	for i, r := range a.Renditions {
		out.Renditions[i] = Rendition{URL: urls(r.StorageKey), Width: r.Width, Height: r.Height, Format: r.Format, ByteSize: r.ByteSize}
	}
	return out
}

// ---- imports and audit ----------------------------------------------------------------

// ImportBatch is a staged workbook.
type ImportBatch struct {
	ID          uuid.UUID                     `json:"id"`
	Filename    string                        `json:"filename"`
	SHA256      string                        `json:"sha256"`
	Status      domain.ImportBatchStatus      `json:"status"`
	Counts      map[domain.ImportDecision]int `json:"counts,omitempty"`
	CreatedAt   time.Time                     `json:"createdAt"`
	CommittedAt *time.Time                    `json:"committedAt,omitempty"`
}

// NewImportBatch maps a batch.
func NewImportBatch(b domain.ImportBatch) ImportBatch {
	return ImportBatch{ID: b.ID, Filename: b.Filename, SHA256: b.SHA256, Status: b.Status, Counts: b.Counts, CreatedAt: b.CreatedAt, CommittedAt: b.CommittedAt}
}

// ImportRow is a staged row. Row is 1-based, as spreadsheets number rows.
type ImportRow struct {
	ID              uuid.UUID             `json:"id"`
	Sheet           string                `json:"sheet"`
	Row             int                   `json:"row"`
	Raw             map[string]string     `json:"raw"`
	Proposed        domain.ImportProposal `json:"proposed"`
	Issues          []domain.ImportIssue  `json:"issues"`
	Decision        domain.ImportDecision `json:"decision"`
	TargetProductID *uuid.UUID            `json:"targetProductId,omitempty"`
}

// NewImportRow maps a staged row.
func NewImportRow(r domain.ImportRow) ImportRow {
	return ImportRow{
		ID: r.ID, Sheet: r.SheetName, Row: r.RowIndex + 1, Raw: r.Raw, Proposed: r.Proposed,
		Issues: r.Issues, Decision: r.Decision, TargetProductID: r.TargetProductID,
	}
}

// AuditActor identifies who made a change.
type AuditActor struct {
	UserID *uuid.UUID       `json:"userId,omitempty"`
	Email  *string          `json:"email,omitempty"`
	Kind   domain.ActorKind `json:"kind"`
}

// AuditEntry is one audit log row; Before/After are the stored snapshots.
type AuditEntry struct {
	ID         uuid.UUID       `json:"id"`
	Actor      AuditActor      `json:"actor"`
	Action     string          `json:"action"`
	EntityType string          `json:"entityType"`
	EntityID   *uuid.UUID      `json:"entityId,omitempty"`
	Before     json.RawMessage `json:"before,omitempty"`
	After      json.RawMessage `json:"after,omitempty"`
	IP         *netip.Addr     `json:"ip,omitempty"`
	CreatedAt  time.Time       `json:"createdAt"`
}

// NewAuditEntry maps an audit entry.
func NewAuditEntry(e domain.AuditEntry) AuditEntry {
	return AuditEntry{
		ID: e.ID, Actor: AuditActor{UserID: e.ActorUserID, Email: e.ActorEmail, Kind: e.ActorKind},
		Action: e.Action, EntityType: e.EntityType, EntityID: e.EntityID,
		Before: e.Before, After: e.After, IP: e.IP, CreatedAt: e.CreatedAt,
	}
}
