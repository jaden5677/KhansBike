package domain

import (
	"time"

	"github.com/google/uuid"
)

// AssetStatus tracks an uploaded image through the derivation pipeline. Uploads
// return immediately as pending/processing; renditions and placeholders are
// produced by a background job that flips the asset to ready or failed.
type AssetStatus string

const (
	AssetPending    AssetStatus = "pending"
	AssetProcessing AssetStatus = "processing"
	AssetReady      AssetStatus = "ready"
	AssetFailed     AssetStatus = "failed"
)

// MediaAsset is one uploaded original, identified by the SHA-256 of its bytes.
// Content addressing means re-uploading identical bytes returns the existing
// asset instead of duplicating storage. Blurhash is a tiny LQIP placeholder
// shown while the full image loads.
type MediaAsset struct {
	ID               uuid.UUID
	SHA256           string // hex encoding of the 32-byte digest
	OriginalFilename string
	MIME             string
	ByteSize         int64
	Width            int
	Height           int
	Blurhash         string
	DominantHex      string
	StorageKey       string
	Status           AssetStatus
	FailureReason    *string
	UploadedBy       uuid.UUID
	CreatedAt        time.Time
	Renditions       []Rendition
}

// Rendition is a resized/reformatted derivative of an asset at a target width.
// Widths come from MEDIA_RENDITION_WIDTHS; formats are webp (primary) and jpeg
// (fallback).
type Rendition struct {
	ID         uuid.UUID
	AssetID    uuid.UUID
	Width      int
	Height     int
	Format     string // "webp" | "jpeg"
	StorageKey string
	ByteSize   int64
}

// MediaRole is how an asset is used on a product. swatch attaches to a specific
// variant and renders on that variant's colour chip.
type MediaRole string

const (
	MediaHero    MediaRole = "hero"
	MediaGallery MediaRole = "gallery"
	MediaDetail  MediaRole = "detail"
	MediaSwatch  MediaRole = "swatch"
)

// ProductMedia binds an asset to a product (and optionally a single variant) in
// a given role and position. A nil VariantID means the association applies to
// the product as a whole.
type ProductMedia struct {
	ID        uuid.UUID
	ProductID uuid.UUID
	VariantID *uuid.UUID
	AssetID   uuid.UUID
	Asset     *MediaAsset
	Role      MediaRole
	Position  int
	AltText   *string
}
