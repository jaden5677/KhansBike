// Package media stores image blobs and derives web renditions from uploads.
//
// Storage is content-addressed for originals (keyed by SHA-256, so identical
// uploads share one blob) and deterministic for renditions (keyed by asset id,
// width and format). Only renditions are ever served publicly: originals keep
// whatever metadata the camera wrote, including GPS coordinates. Rendition keys
// use the asset id rather than the content hash so that a public rendition URL
// never reveals the original's key.
package media

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ErrBlobNotFound means no blob exists under the requested key.
var ErrBlobNotFound = errors.New("media: blob not found")

// BlobStore is where image bytes live: the local filesystem (MEDIA_BACKEND=fs)
// or Cloudflare R2 (MEDIA_BACKEND=r2).
type BlobStore interface {
	// Put stores size bytes from r under key, replacing any existing blob.
	Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error
	// Open returns the blob under key, or ErrBlobNotFound.
	Open(ctx context.Context, key string) (io.ReadSeekCloser, error)
}

// OriginalKey is the private key of an uploaded original, fanned out by the
// first byte of its digest so no directory grows unboundedly.
func OriginalKey(sha256Hex string) string {
	return "originals/" + sha256Hex[:2] + "/" + sha256Hex
}

// RenditionKey is the public key of one derived rendition.
func RenditionKey(assetID string, width int, format string) string {
	return fmt.Sprintf("%s%s/%d.%s", renditionPrefix, assetID, width, format)
}

const renditionPrefix = "renditions/"

// IsPublicKey reports whether a key may be served to anonymous clients. Only
// renditions qualify; originals never do.
func IsPublicKey(key string) bool {
	return strings.HasPrefix(key, renditionPrefix) && fs.ValidPath(key)
}

// FSStore keeps blobs under a root directory on the local disk.
type FSStore struct {
	root string
}

// NewFSStore creates the root directory if needed.
func NewFSStore(root string) (*FSStore, error) {
	if err := os.MkdirAll(root, 0o750); err != nil {
		return nil, fmt.Errorf("media: create root %s: %w", root, err)
	}
	return &FSStore{root: root}, nil
}

// path maps a key to a file path, refusing anything that could escape root.
func (s *FSStore) path(key string) (string, error) {
	if !fs.ValidPath(key) || key == "." {
		return "", fmt.Errorf("media: invalid key %q", key)
	}
	return filepath.Join(s.root, filepath.FromSlash(key)), nil
}

// Put writes to a temporary file and renames it into place, so a reader never
// sees a half-written blob and a crash never leaves one behind.
func (s *FSStore) Put(ctx context.Context, key string, r io.Reader, _ int64, _ string) error {
	p, err := s.path(key)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	dir := filepath.Dir(p)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("media: create %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".upload-*")
	if err != nil {
		return fmt.Errorf("media: create temp file: %w", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }() // no-op after a successful rename
	if _, err := io.Copy(tmp, r); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("media: write %s: %w", key, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("media: write %s: %w", key, err)
	}
	if err := os.Rename(tmp.Name(), p); err != nil {
		return fmt.Errorf("media: store %s: %w", key, err)
	}
	return nil
}

// Open opens the blob under key.
func (s *FSStore) Open(_ context.Context, key string) (io.ReadSeekCloser, error) {
	p, err := s.path(key)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(p)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrBlobNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("media: open %s: %w", key, err)
	}
	return f, nil
}
