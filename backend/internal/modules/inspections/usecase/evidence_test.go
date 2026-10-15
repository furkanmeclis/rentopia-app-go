package usecase

import (
	"encoding/binary"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
)

// tiffWithDate is a little-endian TIFF block with only DateTimeOriginal
// (no offset) and a southern / western GPS position.
func tiffWithDate() []byte {
	le := binary.LittleEndian
	out := []byte("II")
	out = le.AppendUint16(out, 42)
	out = le.AppendUint32(out, 8)
	// IFD0 at 8: Exif pointer + GPS pointer.
	exifAt, gpsAt := 8+2+2*12+4, 8+2+2*12+4+2+12+4
	dataAt := gpsAt + 2 + 4*12 + 4
	out = le.AppendUint16(out, 2)
	for _, e := range [][3]uint32{{0x8769, 4, uint32(exifAt)}, {0x8825, 4, uint32(gpsAt)}} {
		out = le.AppendUint16(out, uint16(e[0]))
		out = le.AppendUint16(out, uint16(e[1]))
		out = le.AppendUint32(out, 1)
		out = le.AppendUint32(out, e[2])
	}
	out = le.AppendUint32(out, 0)
	date := append([]byte("2025:12:31 23:59:58"), 0)
	out = le.AppendUint16(out, 1)
	out = le.AppendUint16(out, 0x9003)
	out = le.AppendUint16(out, 2)
	out = le.AppendUint32(out, uint32(len(date)))
	out = le.AppendUint32(out, uint32(dataAt))
	out = le.AppendUint32(out, 0)
	rat := func(v ...uint32) []byte {
		b := []byte{}
		for _, x := range v {
			b = le.AppendUint32(b, x)
			b = le.AppendUint32(b, 1)
		}
		return b
	}
	latAt := dataAt + len(date)
	lngAt := latAt + 24
	out = le.AppendUint16(out, 4)
	for _, e := range []struct {
		tag, typ uint16
		count    uint32
		value    []byte
	}{
		{0x0001, 2, 2, []byte{'S', 0, 0, 0}}, {0x0002, 5, 3, le.AppendUint32(nil, uint32(latAt))},
		{0x0003, 2, 2, []byte{'W', 0, 0, 0}}, {0x0004, 5, 3, le.AppendUint32(nil, uint32(lngAt))},
	} {
		out = le.AppendUint16(out, e.tag)
		out = le.AppendUint16(out, e.typ)
		out = le.AppendUint32(out, e.count)
		out = append(out, e.value...)
	}
	out = le.AppendUint32(out, 0)
	out = append(out, date...)
	out = append(out, rat(33, 30, 0)...)
	out = append(out, rat(70, 15, 0)...)
	return out
}

func TestReadEXIF(t *testing.T) {
	t.Parallel()
	tiff := tiffWithDate()
	jpeg := append([]byte{0xFF, 0xD8, 0xFF, 0xE1, byte((len(tiff) + 8) >> 8), byte(len(tiff) + 8)}, append([]byte("Exif\x00\x00"), tiff...)...)
	jpeg = append(jpeg, 0xFF, 0xD9)
	ex := ReadEXIF(jpeg)
	if ex.TakenAt != nil || ex.TakenLocal != "2025:12:31 23:59:58" {
		t.Fatalf("without an offset the local time stays raw: %+v", ex)
	}
	if ex.Lat == nil || *ex.Lat != -33.5 || ex.Lng == nil || *ex.Lng != -70.25 {
		t.Fatalf("gps %v %v", ex.Lat, ex.Lng)
	}
	// WebP EXIF chunk.
	chunk := append([]byte("EXIF"), binary.LittleEndian.AppendUint32(nil, uint32(len(tiff)))...)
	chunk = append(chunk, tiff...)
	if len(tiff)%2 == 1 {
		chunk = append(chunk, 0)
	}
	webp := append([]byte("RIFF"), binary.LittleEndian.AppendUint32(nil, uint32(4+len(chunk)))...)
	webp = append(append(webp, []byte("WEBP")...), chunk...)
	if got := ReadEXIF(webp); got.TakenLocal != ex.TakenLocal {
		t.Fatalf("webp exif %+v", got)
	}
	if !ReadEXIF([]byte("not an image")).Empty() || !ReadEXIF(jpeg[:20]).Empty() {
		t.Fatal("unreadable input yields nothing")
	}
}

func TestSniffImage(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		head []byte
		mime string
		ok   bool
	}{
		"jpeg": {[]byte{0xFF, 0xD8, 0xFF, 0xE0, 0, 0}, "image/jpeg", true},
		"png":  {[]byte("\x89PNG\r\n\x1a\n0000"), "image/png", true},
		"webp": {append([]byte("RIFF\x00\x00\x00\x00WEBPVP8 "), make([]byte, 8)...), "image/webp", true},
		"heic": {[]byte("\x00\x00\x00\x18ftypheic\x00\x00\x00\x00"), "image/heic", true},
		"heif": {[]byte("\x00\x00\x00\x18ftypmif1\x00\x00\x00\x00"), "image/heif", true},
		"pdf":  {[]byte("%PDF-1.7\n"), "", false},
		"svg":  {[]byte("<svg xmlns='http://www.w3.org/2000/svg'/>"), "", false},
	}
	for name, c := range cases {
		mime, _, ok := SniffImage(c.head)
		if ok != c.ok || mime != c.mime {
			t.Errorf("%s: %q %v", name, mime, ok)
		}
	}
}

func TestMediaSigner(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	m := NewMediaSigner("secret")
	m.now = func() time.Time { return now }
	id := uuid.New()
	token, exp := m.Sign(7, MediaPhoto, id)
	if exp.Sub(now) != MediaTTL {
		t.Fatalf("expiry %v", exp)
	}
	got, err := m.Verify(token)
	if err != nil || got.OrgID != 7 || got.ID != id || got.Kind != MediaPhoto {
		t.Fatalf("verify %+v %v", got, err)
	}
	other := NewMediaSigner("other")
	other.now = m.now
	if _, err := other.Verify(token); err == nil {
		t.Fatal("a token of another key must fail")
	}
	// A payload naming another organization does not keep the signature.
	forged, _ := json.Marshal(map[string]any{"o": 8, "k": MediaPhoto, "u": id, "e": exp.Unix()})
	if _, err := m.Verify(string(forged) + token[len(token)-10:]); err == nil {
		t.Fatal("a forged payload must fail")
	}
	m.now = func() time.Time { return now.Add(MediaTTL + time.Second) }
	if _, err := m.Verify(token); err == nil {
		t.Fatal("an expired token must fail")
	}
}

func TestCheckStrokes(t *testing.T) {
	t.Parallel()
	pts := [][]float64{}
	for i := 0; i < 10; i++ {
		pts = append(pts, []float64{float64(i), 1, 0.4, float64(i * 10)})
	}
	good, _ := json.Marshal(map[string]any{"width": 300, "height": 100, "strokes": [][][]float64{pts}})
	if _, n, err := checkStrokes(good); err != nil || n != 10 {
		t.Fatalf("good strokes: %d %v", n, err)
	}
	bad := map[string]any{
		"short":    map[string]any{"width": 300, "height": 100, "strokes": [][][]float64{pts[:2]}},
		"pressure": map[string]any{"width": 300, "height": 100, "strokes": [][][]float64{append(pts, []float64{1, 1, 3})}},
		"canvas":   map[string]any{"width": 0, "height": 100, "strokes": [][][]float64{pts}},
	}
	for name, v := range bad {
		raw, _ := json.Marshal(v)
		if _, _, err := checkStrokes(raw); err == nil {
			t.Errorf("%s strokes must fail", name)
		}
	}
}
