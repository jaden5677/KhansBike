package media

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/draw"
)

// Phone cameras usually store photos in sensor orientation and record how to
// rotate them in the EXIF Orientation tag; Go's JPEG decoder ignores that tag,
// so without this step portrait product photos would render sideways. Only
// the one tag is needed, so this reads it directly rather than pulling in a
// general EXIF library. Every offset is bounds-checked: the bytes are
// untrusted upload data.

const exifOrientationTag = 0x0112

// jpegOrientation returns the EXIF orientation (1-8) of a JPEG, or 1 (upright)
// when the data is not a JPEG or carries no readable orientation.
func jpegOrientation(data []byte) int {
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return 1
	}
	i := 2
	for i+4 <= len(data) {
		if data[i] != 0xFF {
			return 1
		}
		marker := data[i+1]
		switch {
		case marker == 0xFF: // fill byte before a marker
			i++
			continue
		case marker == 0xDA || marker == 0xD9: // start of scan / end of image: no EXIF follows
			return 1
		case marker >= 0xD0 && marker <= 0xD7, marker == 0x01: // markers without a length
			i += 2
			continue
		}
		length := int(binary.BigEndian.Uint16(data[i+2:]))
		if length < 2 || i+2+length > len(data) {
			return 1
		}
		segment := data[i+4 : i+2+length]
		if marker == 0xE1 && bytes.HasPrefix(segment, []byte("Exif\x00\x00")) {
			if o := tiffOrientation(segment[6:]); o != 0 {
				return o
			}
			return 1
		}
		i += 2 + length
	}
	return 1
}

// tiffOrientation reads the Orientation tag from IFD0 of a TIFF structure,
// returning 0 when absent or malformed.
func tiffOrientation(t []byte) int {
	if len(t) < 8 {
		return 0
	}
	var order binary.ByteOrder
	switch string(t[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return 0
	}
	if order.Uint16(t[2:]) != 42 {
		return 0
	}
	off := int64(order.Uint32(t[4:]))
	if off < 8 || off+2 > int64(len(t)) {
		return 0
	}
	entries := int64(order.Uint16(t[off:]))
	for k := int64(0); k < entries; k++ {
		e := off + 2 + 12*k
		if e+12 > int64(len(t)) {
			return 0
		}
		if order.Uint16(t[e:]) == exifOrientationTag {
			if v := int(order.Uint16(t[e+8:])); v >= 1 && v <= 8 {
				return v
			}
			return 0
		}
	}
	return 0
}

// orient returns img transformed so that it displays upright for EXIF
// orientation o (1 = already upright). Orientations 5-8 swap width and height.
func orient(img image.Image, o int) image.Image {
	if o < 2 || o > 8 {
		return img
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	src := image.NewNRGBA(image.Rect(0, 0, w, h))
	draw.Draw(src, src.Bounds(), img, b.Min, draw.Src)

	dw, dh := w, h
	if o >= 5 {
		dw, dh = h, w
	}
	dst := image.NewNRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < dh; y++ {
		for x := 0; x < dw; x++ {
			var sx, sy int
			switch o {
			case 2: // mirror horizontally
				sx, sy = w-1-x, y
			case 3: // rotate 180
				sx, sy = w-1-x, h-1-y
			case 4: // mirror vertically
				sx, sy = x, h-1-y
			case 5: // transpose
				sx, sy = y, x
			case 6: // rotate 90 clockwise
				sx, sy = y, h-1-x
			case 7: // transverse
				sx, sy = w-1-y, h-1-x
			case 8: // rotate 90 counter-clockwise
				sx, sy = w-1-y, x
			}
			si, di := src.PixOffset(sx, sy), dst.PixOffset(x, y)
			copy(dst.Pix[di:di+4], src.Pix[si:si+4])
		}
	}
	return dst
}
