package usecase

import (
	"bytes"
	"encoding/binary"
	"math"
	"net/http"
	"strings"
	"time"
)

// SniffImage returns the image type of the first bytes of a file (JPEG,
// PNG, WebP or HEIC / HEIF) and its file extension; ok is false for
// anything else (the declared type is never trusted).
func SniffImage(head []byte) (mime, ext string, ok bool) {
	if len(head) >= 12 && string(head[4:8]) == "ftyp" {
		switch string(head[8:12]) {
		case "heic", "heix", "hevc", "hevx", "heim", "heis":
			return "image/heic", "heic", true
		case "mif1", "msf1":
			return "image/heif", "heif", true
		}
	}
	m := strings.ToLower(http.DetectContentType(head))
	if i := strings.Index(m, ";"); i >= 0 {
		m = strings.TrimSpace(m[:i])
	}
	switch m {
	case "image/jpeg":
		return m, "jpg", true
	case "image/png":
		return m, "png", true
	case "image/webp":
		return m, "webp", true
	}
	return "", "", false
}

// EXIF is what the evidence keeps of a file's EXIF block: the device's
// capture time and GPS position (kept apart from the server time).
type EXIF struct {
	// TakenAt is set when the file carries an offset (OffsetTimeOriginal);
	// TakenLocal is the raw local "2006:01:02 15:04:05" otherwise.
	TakenAt    *time.Time
	TakenLocal string
	Lat, Lng   *float64
	Make       string
	Model      string
}

// Empty reports whether nothing was found.
func (e EXIF) Empty() bool {
	return e.TakenAt == nil && e.TakenLocal == "" && e.Lat == nil && e.Make == "" && e.Model == ""
}

// ReadEXIF parses the EXIF block of a JPEG (APP1) or WebP (EXIF chunk)
// head; anything it cannot read is skipped (never an error).
func ReadEXIF(head []byte) EXIF {
	if tiff := jpegTIFF(head); tiff != nil {
		return parseTIFF(tiff)
	}
	if tiff := webpTIFF(head); tiff != nil {
		return parseTIFF(tiff)
	}
	return EXIF{}
}

func jpegTIFF(b []byte) []byte {
	if len(b) < 4 || b[0] != 0xFF || b[1] != 0xD8 {
		return nil
	}
	i := 2
	for i+4 <= len(b) {
		if b[i] != 0xFF {
			return nil
		}
		marker := b[i+1]
		if marker == 0xD9 || marker == 0xDA { // end / start of scan
			return nil
		}
		size := int(binary.BigEndian.Uint16(b[i+2 : i+4]))
		if size < 2 || i+2+size > len(b) {
			return nil
		}
		seg := b[i+4 : i+2+size]
		if marker == 0xE1 && len(seg) > 6 && string(seg[:6]) == "Exif\x00\x00" {
			return seg[6:]
		}
		i += 2 + size
	}
	return nil
}

func webpTIFF(b []byte) []byte {
	if len(b) < 12 || string(b[:4]) != "RIFF" || string(b[8:12]) != "WEBP" {
		return nil
	}
	i := 12
	for i+8 <= len(b) {
		id := string(b[i : i+4])
		size := int(binary.LittleEndian.Uint32(b[i+4 : i+8]))
		if size < 0 || i+8+size > len(b) {
			return nil
		}
		if id == "EXIF" {
			data := b[i+8 : i+8+size]
			return bytes.TrimPrefix(data, []byte("Exif\x00\x00"))
		}
		i += 8 + size + size%2
	}
	return nil
}

type tiffReader struct {
	b  []byte
	bo binary.ByteOrder
}

type ifdEntry struct {
	tag, typ uint16
	count    uint32
	value    []byte // the raw value bytes (inline or at the offset)
}

func parseTIFF(b []byte) EXIF {
	out := EXIF{}
	if len(b) < 8 {
		return out
	}
	var r tiffReader
	switch string(b[:2]) {
	case "II":
		r = tiffReader{b: b, bo: binary.LittleEndian}
	case "MM":
		r = tiffReader{b: b, bo: binary.BigEndian}
	default:
		return out
	}
	if r.bo.Uint16(b[2:4]) != 42 {
		return out
	}
	ifd0 := r.entries(int(r.bo.Uint32(b[4:8])))
	var exifIFD, gpsIFD []ifdEntry
	for _, e := range ifd0 {
		switch e.tag {
		case 0x010F:
			out.Make = e.ascii()
		case 0x0110:
			out.Model = e.ascii()
		case 0x8769:
			if off, ok := r.long(e); ok {
				exifIFD = r.entries(off)
			}
		case 0x8825:
			if off, ok := r.long(e); ok {
				gpsIFD = r.entries(off)
			}
		}
	}
	var original, offset string
	for _, e := range exifIFD {
		switch e.tag {
		case 0x9003:
			original = e.ascii()
		case 0x9011:
			offset = e.ascii()
		}
	}
	if original != "" {
		if offset != "" {
			if t, err := time.Parse("2006:01:02 15:04:05-07:00", original+offset); err == nil {
				u := t.UTC()
				out.TakenAt = &u
			}
		}
		if _, err := time.Parse("2006:01:02 15:04:05", original); err == nil {
			out.TakenLocal = original
		}
	}
	var latRef, lngRef string
	var lat, lng []float64
	for _, e := range gpsIFD {
		switch e.tag {
		case 0x0001:
			latRef = e.ascii()
		case 0x0002:
			lat = r.rationals(e)
		case 0x0003:
			lngRef = e.ascii()
		case 0x0004:
			lng = r.rationals(e)
		}
	}
	if v, ok := dms(lat, latRef, "S", 90); ok {
		out.Lat = &v
	}
	if v, ok := dms(lng, lngRef, "W", 180); ok {
		out.Lng = &v
	}
	return out
}

func dms(parts []float64, ref, negative string, limit float64) (float64, bool) {
	if len(parts) != 3 {
		return 0, false
	}
	v := parts[0] + parts[1]/60 + parts[2]/3600
	if strings.EqualFold(strings.TrimSpace(ref), negative) {
		v = -v
	}
	if math.IsNaN(v) || math.Abs(v) > limit {
		return 0, false
	}
	return math.Round(v*1e6) / 1e6, true
}

var typeSizes = map[uint16]int{1: 1, 2: 1, 3: 2, 4: 4, 5: 8, 7: 1, 9: 4, 10: 8}

func (r tiffReader) entries(off int) []ifdEntry {
	if off <= 0 || off+2 > len(r.b) {
		return nil
	}
	n := int(r.bo.Uint16(r.b[off : off+2]))
	if n > 512 {
		return nil
	}
	out := make([]ifdEntry, 0, n)
	for i := 0; i < n; i++ {
		p := off + 2 + i*12
		if p+12 > len(r.b) {
			break
		}
		e := ifdEntry{tag: r.bo.Uint16(r.b[p : p+2]), typ: r.bo.Uint16(r.b[p+2 : p+4]), count: r.bo.Uint32(r.b[p+4 : p+8])}
		size, ok := typeSizes[e.typ]
		if !ok || e.count > 1<<16 {
			continue
		}
		total := size * int(e.count)
		if total <= 4 {
			e.value = r.b[p+8 : p+8+total]
		} else {
			vo := int(r.bo.Uint32(r.b[p+8 : p+12]))
			if vo < 0 || vo+total > len(r.b) {
				continue
			}
			e.value = r.b[vo : vo+total]
		}
		out = append(out, e)
	}
	return out
}

func (e ifdEntry) ascii() string {
	if e.typ != 2 {
		return ""
	}
	return strings.TrimSpace(strings.TrimRight(string(e.value), "\x00"))
}

func (r tiffReader) long(e ifdEntry) (int, bool) {
	switch {
	case e.typ == 4 && len(e.value) >= 4:
		return int(r.bo.Uint32(e.value[:4])), true
	case e.typ == 3 && len(e.value) >= 2:
		return int(r.bo.Uint16(e.value[:2])), true
	}
	return 0, false
}

func (r tiffReader) rationals(e ifdEntry) []float64 {
	if e.typ != 5 {
		return nil
	}
	out := []float64{}
	for i := 0; i+8 <= len(e.value); i += 8 {
		num := float64(r.bo.Uint32(e.value[i : i+4]))
		den := float64(r.bo.Uint32(e.value[i+4 : i+8]))
		if den == 0 {
			return nil
		}
		out = append(out, num/den)
	}
	return out
}
