package media

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"slices"
	"strings"
	"testing"
)

// withOrientation splices an EXIF APP1 segment carrying the given orientation
// into a JPEG, right after its SOI marker.
func withOrientation(t *testing.T, jpg []byte, o uint16, order binary.ByteOrder) []byte {
	t.Helper()
	var tiff bytes.Buffer
	if order == binary.LittleEndian {
		tiff.WriteString("II")
	} else {
		tiff.WriteString("MM")
	}
	_ = binary.Write(&tiff, order, uint16(42))
	_ = binary.Write(&tiff, order, uint32(8)) // IFD0 right after the header
	_ = binary.Write(&tiff, order, uint16(1)) // one entry
	_ = binary.Write(&tiff, order, uint16(exifOrientationTag))
	_ = binary.Write(&tiff, order, uint16(3)) // SHORT
	_ = binary.Write(&tiff, order, uint32(1))
	_ = binary.Write(&tiff, order, o)
	_ = binary.Write(&tiff, order, uint16(0)) // value padding
	_ = binary.Write(&tiff, order, uint32(0)) // no next IFD

	payload := append([]byte("Exif\x00\x00"), tiff.Bytes()...)
	seg := []byte{0xFF, 0xE1, 0, 0}
	binary.BigEndian.PutUint16(seg[2:], uint16(len(payload)+2))
	seg = append(seg, payload...)

	out := append([]byte{}, jpg[:2]...)
	out = append(out, seg...)
	return append(out, jpg[2:]...)
}

// testImage is w×h with a distinct colour in the top-left pixel, so tests can
// track where that corner ends up.
func testImage(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.NRGBA{0, 0, 255, 255})
		}
	}
	img.Set(0, 0, color.NRGBA{255, 0, 0, 255})
	return img
}

func encodeJPEG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestJPEGOrientation(t *testing.T) {
	base := encodeJPEG(t, testImage(8, 4))
	if got := jpegOrientation(base); got != 1 {
		t.Errorf("no EXIF: orientation = %d, want 1", got)
	}
	for _, order := range []binary.ByteOrder{binary.LittleEndian, binary.BigEndian} {
		if got := jpegOrientation(withOrientation(t, base, 6, order)); got != 6 {
			t.Errorf("%v: orientation = %d, want 6", order, got)
		}
	}
	if got := jpegOrientation([]byte("not a jpeg at all")); got != 1 {
		t.Errorf("garbage: orientation = %d, want 1", got)
	}
	// A truncated EXIF segment must not panic or misread.
	trunc := withOrientation(t, base, 6, binary.LittleEndian)[:30]
	_ = jpegOrientation(trunc)
}

func TestOrientTransforms(t *testing.T) {
	src := testImage(3, 2) // red at top-left
	tests := []struct {
		o          int
		wantW      int
		wantH      int
		redX, redY int // where the red top-left pixel must end up
	}{
		{1, 3, 2, 0, 0},
		{2, 3, 2, 2, 0},
		{3, 3, 2, 2, 1},
		{4, 3, 2, 0, 1},
		{5, 2, 3, 0, 0},
		{6, 2, 3, 1, 0},
		{7, 2, 3, 1, 2},
		{8, 2, 3, 0, 2},
	}
	for _, tc := range tests {
		got := orient(src, tc.o)
		b := got.Bounds()
		if b.Dx() != tc.wantW || b.Dy() != tc.wantH {
			t.Errorf("o=%d: size %dx%d, want %dx%d", tc.o, b.Dx(), b.Dy(), tc.wantW, tc.wantH)
			continue
		}
		if r, _, _, _ := got.At(tc.redX, tc.redY).RGBA(); r>>8 != 255 {
			t.Errorf("o=%d: red pixel not at (%d,%d)", tc.o, tc.redX, tc.redY)
		}
	}
}

func TestTargetWidths(t *testing.T) {
	widths := []int{320, 640, 960, 1280, 1920}
	if got := targetWidths(1000, widths); !slices.Equal(got, []int{960, 640, 320}) {
		t.Errorf("targetWidths(1000) = %v", got)
	}
	if got := targetWidths(200, widths); !slices.Equal(got, []int{200}) {
		t.Errorf("targetWidths(200) = %v (must not upscale)", got)
	}
}

func TestProcessProducesUprightRenditions(t *testing.T) {
	// 40x20 landscape pixels tagged "rotate 90° clockwise" is a 20x40 portrait.
	data := withOrientation(t, encodeJPEG(t, testImage(40, 20)), 6, binary.LittleEndian)
	p, err := Process(data, 1_000_000, []int{10, 16, 100})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if p.Width != 20 || p.Height != 40 {
		t.Errorf("upright size = %dx%d, want 20x40", p.Width, p.Height)
	}
	var formats []string
	for _, r := range p.Renditions {
		formats = append(formats, r.Format)
		if r.Height != r.Width*2 {
			t.Errorf("%s %dpx: height %d breaks the 1:2 aspect ratio", r.Format, r.Width, r.Height)
		}
		if SniffType(r.Data) != r.ContentType {
			t.Errorf("%s %dpx: encoded bytes sniff as %q", r.Format, r.Width, SniffType(r.Data))
		}
	}
	if !slices.Equal(formats, []string{"webp", "jpeg", "webp", "jpeg"}) { // widths 16 and 10
		t.Errorf("renditions = %v", formats)
	}
	if p.Blurhash == "" || !strings.HasPrefix(p.DominantHex, "#") {
		t.Errorf("placeholders missing: blurhash=%q dominant=%q", p.Blurhash, p.DominantHex)
	}
}

func TestProcessRejectsPixelBombsAndGarbage(t *testing.T) {
	var buf bytes.Buffer
	_ = png.Encode(&buf, testImage(100, 100))
	if _, err := Process(buf.Bytes(), 5_000, []int{50}); !errors.Is(err, ErrTooManyPixels) {
		t.Errorf("pixel limit: err = %v, want ErrTooManyPixels", err)
	}
	if _, err := Process([]byte("definitely not an image"), 1_000_000, []int{50}); !errors.Is(err, ErrUnsupportedImage) {
		t.Errorf("garbage: err = %v, want ErrUnsupportedImage", err)
	}
}

func TestSniffType(t *testing.T) {
	if got := SniffType(encodeJPEG(t, testImage(4, 4))); got != "image/jpeg" {
		t.Errorf("jpeg sniffed as %q", got)
	}
	if got := SniffType([]byte("GIF89a......")); got != "" {
		t.Errorf("gif accepted as %q", got)
	}
}

func TestFSStore(t *testing.T) {
	s, err := NewFSStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	key := RenditionKey("asset-1", 320, "webp")
	if err := s.Put(ctx, key, strings.NewReader("hello"), 5, "image/webp"); err != nil {
		t.Fatalf("Put: %v", err)
	}
	f, err := s.Open(ctx, key)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	b, _ := io.ReadAll(f)
	_ = f.Close()
	if string(b) != "hello" {
		t.Errorf("read back %q", b)
	}
	if _, err := s.Open(ctx, "renditions/missing/1.webp"); !errors.Is(err, ErrBlobNotFound) {
		t.Errorf("missing blob: err = %v", err)
	}
	for _, bad := range []string{"../escape", "/abs/path", "a/../../b", ""} {
		if err := s.Put(ctx, bad, strings.NewReader("x"), 1, ""); err == nil {
			t.Errorf("Put accepted unsafe key %q", bad)
		}
	}
}

func TestIsPublicKey(t *testing.T) {
	if !IsPublicKey(RenditionKey("abc", 640, "jpeg")) {
		t.Error("rendition key not public")
	}
	for _, k := range []string{OriginalKey(strings.Repeat("ab", 32)), "renditions/../originals/x", "other/file"} {
		if IsPublicKey(k) {
			t.Errorf("%q reported public", k)
		}
	}
}
