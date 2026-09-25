package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"

	"github.com/khansbikezone/bikezone-api/internal/domain"
	"github.com/khansbikezone/bikezone-api/internal/jobs"
	"github.com/khansbikezone/bikezone-api/internal/media"
	"github.com/khansbikezone/bikezone-api/internal/store"
	"github.com/khansbikezone/bikezone-api/internal/store/gen"
)

// Media ingests image uploads and derives their renditions. An upload returns
// as soon as the original is safely stored (status pending); the resizing and
// encoding happen in a background job, which flips the asset to ready or
// failed.
type Media struct {
	store     *store.Store
	blobs     media.BlobStore
	cfg       MediaConfig
	publicURL string // base URL that rendition keys are appended to
}

// MediaConfig carries the MEDIA_* settings.
type MediaConfig struct {
	MaxUploadBytes int64
	MaxPixels      int64
	Widths         []int
	// PublicBaseURL is where renditions are served from: the API's own /media/
	// route, or the R2 bucket's public domain.
	PublicBaseURL string
}

// NewMedia builds the media service.
func NewMedia(st *store.Store, blobs media.BlobStore, cfg MediaConfig) *Media {
	return &Media{store: st, blobs: blobs, cfg: cfg, publicURL: strings.TrimSuffix(cfg.PublicBaseURL, "/") + "/"}
}

// URL is the public address of a stored rendition.
func (m *Media) URL(key string) string { return m.publicURL + key }

// maxFilenameLen bounds the recorded original filename.
const maxFilenameLen = 255

// processMediaPayload is the process_media job payload.
type processMediaPayload struct {
	AssetID uuid.UUID `json:"assetId"`
}

// Upload stores an image and schedules its processing. It streams the body to
// a temporary file while hashing it, so a 25 MB upload never sits in memory,
// then checks the bytes: the type is sniffed from content (the client's
// Content-Type and filename are not trusted) and the dimensions are read from
// the header to refuse decompression bombs before anything decodes them.
// Identical bytes resolve to the existing asset; created reports whether a new
// asset was made.
func (m *Media) Upload(ctx context.Context, filename string, body io.Reader) (asset domain.MediaAsset, created bool, err error) {
	tmp, err := os.CreateTemp("", "bikezone-upload-*")
	if err != nil {
		return asset, false, fmt.Errorf("create temp file: %w", err)
	}
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
	}()

	hash := sha256.New()
	// Read one byte past the limit to detect oversize uploads.
	n, err := io.Copy(io.MultiWriter(tmp, hash), io.LimitReader(body, m.cfg.MaxUploadBytes+1))
	if err != nil {
		return asset, false, fmt.Errorf("receive upload: %w", err)
	}
	if n == 0 {
		return asset, false, domain.Invalid("file", "is empty")
	}
	if n > m.cfg.MaxUploadBytes {
		return asset, false, domain.Invalid("file", "is larger than the %d byte limit", m.cfg.MaxUploadBytes)
	}
	digest := hash.Sum(nil)
	shaHex := hex.EncodeToString(digest)

	if row, err := m.store.GetAssetBySHA(ctx, digest); err == nil {
		return store.MediaAsset(row), false, nil // already uploaded: dedupe
	} else if !errors.Is(err, domain.ErrNotFound) {
		return asset, false, err
	}

	head := make([]byte, 512)
	if _, err := tmp.ReadAt(head, 0); err != nil && !errors.Is(err, io.EOF) {
		return asset, false, fmt.Errorf("read upload: %w", err)
	}
	mime := media.SniffType(head)
	if mime == "" {
		return asset, false, domain.Invalid("file", "must be a JPEG, PNG or WebP image")
	}
	cfg, err := media.DecodeConfig(io.NewSectionReader(tmp, 0, n))
	if err != nil {
		return asset, false, domain.Invalid("file", "is not a readable image")
	}
	if int64(cfg.Width)*int64(cfg.Height) > m.cfg.MaxPixels {
		return asset, false, domain.Invalid("file", "is %dx%d pixels, above the %d pixel limit", cfg.Width, cfg.Height, m.cfg.MaxPixels)
	}

	key := media.OriginalKey(shaHex)
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return asset, false, fmt.Errorf("rewind upload: %w", err)
	}
	if err := m.blobs.Put(ctx, key, tmp, n, mime); err != nil {
		return asset, false, err
	}

	uploader := domain.ActorFrom(ctx).UserID
	err = m.store.InTx(ctx, func(q *store.Queries) error {
		row, err := q.InsertAsset(ctx, gen.InsertAssetParams{
			ID:               domain.NewID(),
			Sha256:           digest,
			OriginalFilename: cleanFilename(filename),
			Mime:             mime,
			ByteSize:         n,
			Width:            store.Int4(cfg.Width),
			Height:           store.Int4(cfg.Height),
			StorageKey:       key,
			Status:           gen.AssetStatusPending,
			UploadedBy:       uploader,
		})
		if err != nil {
			return err
		}
		asset = store.MediaAsset(row)
		if asset.Status != domain.AssetPending {
			return nil // a concurrent identical upload won; it scheduled the job
		}
		created = true
		if err := q.Enqueue(ctx, JobProcessMedia, processMediaPayload{AssetID: asset.ID}); err != nil {
			return err
		}
		return q.Audit(ctx, "media.upload", "media_asset", &asset.ID, nil, map[string]any{
			"filename": asset.OriginalFilename, "bytes": n, "mime": mime,
		})
	})
	return asset, created, err
}

// cleanFilename keeps only the base name of what the client sent, bounded.
func cleanFilename(name string) string {
	name = filepath.Base(strings.ReplaceAll(strings.TrimSpace(name), `\`, "/"))
	if name == "." || name == "/" || name == "" {
		name = "upload"
	}
	if len(name) > maxFilenameLen {
		name = name[:maxFilenameLen]
	}
	return name
}

// Asset returns an asset with its renditions.
func (m *Media) Asset(ctx context.Context, id uuid.UUID) (domain.MediaAsset, error) {
	assets, err := loadAssets(ctx, m.store.Queries, []uuid.UUID{id})
	if err != nil {
		return domain.MediaAsset{}, err
	}
	a, ok := assets[id]
	if !ok {
		return domain.MediaAsset{}, fmt.Errorf("media asset %s: %w", id, domain.ErrNotFound)
	}
	return *a, nil
}

// OpenPublic opens a rendition for serving. Only rendition keys are public:
// originals keep the camera's metadata (GPS included) and are never served.
func (m *Media) OpenPublic(ctx context.Context, key string) (io.ReadSeekCloser, error) {
	if !media.IsPublicKey(key) {
		return nil, fmt.Errorf("media %q: %w", key, domain.ErrNotFound)
	}
	f, err := m.blobs.Open(ctx, key)
	if errors.Is(err, media.ErrBlobNotFound) {
		return nil, fmt.Errorf("media %q: %w", key, domain.ErrNotFound)
	}
	return f, err
}

// ProcessJob is the process_media job handler: decode the original, turn it
// upright, write WebP and JPEG renditions, and record placeholders. An image
// that cannot be decoded is marked failed and not retried.
func (m *Media) ProcessJob(ctx context.Context, payload []byte) error {
	var p processMediaPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return jobs.Permanent(fmt.Errorf("decode payload: %w", err))
	}
	row, err := m.store.GetAssetByID(ctx, p.AssetID)
	if errors.Is(err, domain.ErrNotFound) {
		return jobs.Permanent(fmt.Errorf("asset %s no longer exists", p.AssetID))
	}
	if err != nil {
		return err
	}
	asset := store.MediaAsset(row)
	if asset.Status == domain.AssetReady {
		return nil // already processed (a retried delivery)
	}
	if err := m.store.MarkAssetProcessing(ctx, asset.ID); err != nil {
		return err
	}

	original, err := m.readOriginal(ctx, asset.StorageKey)
	if err != nil {
		return err
	}
	out, err := media.Process(original, m.cfg.MaxPixels, m.cfg.Widths)
	if errors.Is(err, media.ErrUnsupportedImage) || errors.Is(err, media.ErrTooManyPixels) {
		if ferr := m.store.MarkAssetFailed(ctx, gen.MarkAssetFailedParams{ID: asset.ID, FailureReason: store.TextOrNull(err.Error())}); ferr != nil {
			return ferr
		}
		return jobs.Permanent(err)
	}
	if err != nil {
		return err
	}

	// Upload first (slow I/O, outside any transaction), then record the rows
	// atomically. A crash in between just means a retry overwrites the blobs.
	keys := make([]string, len(out.Renditions))
	for i, r := range out.Renditions {
		keys[i] = media.RenditionKey(asset.ID.String(), r.Width, r.Format)
		if err := m.blobs.Put(ctx, keys[i], bytes.NewReader(r.Data), int64(len(r.Data)), r.ContentType); err != nil {
			return err
		}
	}
	return m.store.InTx(ctx, func(q *store.Queries) error {
		for i, r := range out.Renditions {
			if _, err := q.InsertRendition(ctx, gen.InsertRenditionParams{
				ID: domain.NewID(), AssetID: asset.ID, Width: int32(r.Width), Height: int32(r.Height),
				Format: r.Format, StorageKey: keys[i], ByteSize: int64(len(r.Data)),
			}); err != nil {
				return err
			}
		}
		return q.MarkAssetReady(ctx, gen.MarkAssetReadyParams{
			ID: asset.ID, Width: store.Int4(out.Width), Height: store.Int4(out.Height),
			Blurhash: store.TextOrNull(out.Blurhash), DominantHex: store.TextOrNull(out.DominantHex),
		})
	})
}

func (m *Media) readOriginal(ctx context.Context, key string) ([]byte, error) {
	f, err := m.blobs.Open(ctx, key)
	if errors.Is(err, media.ErrBlobNotFound) {
		return nil, jobs.Permanent(fmt.Errorf("original %s is missing from storage", key))
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	b, err := io.ReadAll(io.LimitReader(f, m.cfg.MaxUploadBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read original %s: %w", key, err)
	}
	return b, nil
}
