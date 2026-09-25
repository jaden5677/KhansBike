package media

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	_ "image/png" // register the PNG decoder
	"io"
	"math"
	"net/http"
	"slices"

	"github.com/buckket/go-blurhash"
	"github.com/gen2brain/webp" // WebP encoder; also registers the WebP decoder
	xdraw "golang.org/x/image/draw"
)

// Errors that make an upload permanently unprocessable (retrying cannot help).
var (
	ErrUnsupportedImage = errors.New("media: unsupported or corrupt image")
	ErrTooManyPixels    = errors.New("media: image dimensions exceed the pixel limit")
)

// Supported upload types, keyed by sniffed MIME type.
var supportedTypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/webp": true,
}

// SniffType returns the MIME type of an image from its first bytes, or ""
// when it is not a supported image. The client's Content-Type and filename
// are ignored: they are claims, the bytes are evidence.
func SniffType(head []byte) string {
	ct := http.DetectContentType(head)
	if supportedTypes[ct] {
		return ct
	}
	return ""
}

// DecodeConfig reads only as much of an image as its header needs, to learn
// its dimensions before committing memory to a full decode.
func DecodeConfig(r io.Reader) (image.Config, error) {
	cfg, _, err := image.DecodeConfig(r)
	if err != nil {
		return cfg, fmt.Errorf("%w: %w", ErrUnsupportedImage, err)
	}
	return cfg, nil
}

// Rendition is one encoded derivative of an image.
type Rendition struct {
	Width       int
	Height      int
	Format      string // "webp" or "jpeg"
	ContentType string
	Data        []byte
}

// Processed is everything derived from an original.
type Processed struct {
	Width       int // upright dimensions (after EXIF orientation)
	Height      int
	Blurhash    string // tiny placeholder shown while the image loads
	DominantHex string // average colour, e.g. "#4d4d4d"
	Renditions  []Rendition
}

// Encoding settings: quality ~80 is visually lossless for product photos at
// a fraction of the original size.
const (
	webpQuality = 80
	jpegQuality = 82
	thumbWidth  = 32 // source size for the blurhash and dominant colour
)

// Process decodes an original, refusing pixel bombs before allocating the
// full bitmap, turns it upright, and produces WebP and JPEG renditions at each
// configured width that does not exceed the image (never upscaling; an image
// narrower than every width gets one rendition at its own width).
func Process(data []byte, maxPixels int64, widths []int) (*Processed, error) {
	cfg, err := DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if int64(cfg.Width)*int64(cfg.Height) > maxPixels {
		return nil, fmt.Errorf("%w: %dx%d", ErrTooManyPixels, cfg.Width, cfg.Height)
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnsupportedImage, err)
	}
	img = orient(img, jpegOrientation(data))

	b := img.Bounds()
	out := &Processed{Width: b.Dx(), Height: b.Dy()}
	src := img
	for _, w := range targetWidths(b.Dx(), widths) {
		// Each level is scaled from the previous (larger) one: much faster
		// than always scaling from a 12 MP original, with no visible loss.
		scaled := resize(src, w)
		src = scaled
		for _, enc := range []struct {
			format, contentType string
			encode              func(*bytes.Buffer, image.Image) error
		}{
			{"webp", "image/webp", func(buf *bytes.Buffer, m image.Image) error {
				return webp.Encode(buf, m, webp.Options{Quality: webpQuality})
			}},
			{"jpeg", "image/jpeg", func(buf *bytes.Buffer, m image.Image) error {
				return jpeg.Encode(buf, flatten(m), &jpeg.Options{Quality: jpegQuality})
			}},
		} {
			var buf bytes.Buffer
			if err := enc.encode(&buf, scaled); err != nil {
				return nil, fmt.Errorf("media: encode %s at %dpx: %w", enc.format, w, err)
			}
			out.Renditions = append(out.Renditions, Rendition{
				Width: w, Height: scaled.Bounds().Dy(), Format: enc.format, ContentType: enc.contentType, Data: buf.Bytes(),
			})
		}
	}

	thumb := resize(src, min(thumbWidth, src.Bounds().Dx()))
	if out.Blurhash, err = blurhash.Encode(4, 3, thumb); err != nil {
		return nil, fmt.Errorf("media: blurhash: %w", err)
	}
	out.DominantHex = averageHex(thumb)
	return out, nil
}

// targetWidths returns the configured widths no wider than imgWidth, widest
// first, or just imgWidth when every configured width is wider.
func targetWidths(imgWidth int, widths []int) []int {
	var out []int
	for _, w := range widths {
		if w > 0 && w <= imgWidth && !slices.Contains(out, w) {
			out = append(out, w)
		}
	}
	if len(out) == 0 {
		return []int{imgWidth}
	}
	slices.SortFunc(out, func(a, b int) int { return b - a })
	return out
}

// resize scales src to width, keeping the aspect ratio, with Catmull-Rom
// resampling (sharp downscales without ringing on product edges).
func resize(src image.Image, width int) *image.RGBA {
	b := src.Bounds()
	height := max(1, int(math.Round(float64(b.Dy())*float64(width)/float64(b.Dx()))))
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, b, xdraw.Src, nil)
	return dst
}

// flatten composites an image onto white. JPEG has no alpha channel, and the
// encoder would otherwise turn transparent PNG backgrounds black.
func flatten(m image.Image) image.Image {
	b := m.Bounds()
	dst := image.NewRGBA(b)
	xdraw.Draw(dst, b, image.NewUniform(color.White), image.Point{}, xdraw.Src)
	xdraw.Draw(dst, b, m, b.Min, xdraw.Over)
	return dst
}

// averageHex is the alpha-weighted mean colour of m as "#rrggbb", used as a
// solid placeholder behind an image while it loads.
func averageHex(m image.Image) string {
	var r, g, bl, a uint64
	b := m.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			pr, pg, pb, pa := m.At(x, y).RGBA() // premultiplied, 16-bit
			r, g, bl, a = r+uint64(pr), g+uint64(pg), bl+uint64(pb), a+uint64(pa)
		}
	}
	if a == 0 {
		return "#ffffff"
	}
	// Premultiplied sums divided by total alpha give the straight-alpha mean.
	scale := func(v uint64) uint64 { return v * 255 / a }
	return fmt.Sprintf("#%02x%02x%02x", scale(r), scale(g), scale(bl))
}
