package inspections_test

import "encoding/binary"

// exifTIFF builds a little-endian TIFF block: Make "Rentopia", capture time
// 2026:05:04 10:02:03 +03:00, GPS 41.0125 N 28.9784 E.
func exifTIFF() []byte {
	type entry struct {
		tag, typ uint16
		count    uint32
		data     []byte // > 4 bytes go to the data area
		inline   uint32
	}
	ascii := func(s string) []byte { return append([]byte(s), 0) }
	rationals := func(v ...[2]uint32) []byte {
		out := make([]byte, 0, len(v)*8)
		for _, r := range v {
			out = binary.LittleEndian.AppendUint32(out, r[0])
			out = binary.LittleEndian.AppendUint32(out, r[1])
		}
		return out
	}
	le := binary.LittleEndian
	// Layout: header (8) | IFD0 (2 + 3*12 + 4) | Exif IFD (2 + 2*12 + 4) |
	// GPS IFD (2 + 4*12 + 4) | data.
	ifd0At := 8
	exifAt := ifd0At + 2 + 3*12 + 4
	gpsAt := exifAt + 2 + 2*12 + 4
	dataAt := gpsAt + 2 + 4*12 + 4
	data := []byte{}
	place := func(b []byte) uint32 {
		off := dataAt + len(data)
		data = append(data, b...)
		return uint32(off)
	}
	ifd := func(entries []entry) []byte {
		out := le.AppendUint16(nil, uint16(len(entries)))
		for _, e := range entries {
			out = le.AppendUint16(out, e.tag)
			out = le.AppendUint16(out, e.typ)
			out = le.AppendUint32(out, e.count)
			switch {
			case e.data == nil:
				out = le.AppendUint32(out, e.inline)
			case len(e.data) <= 4:
				v := make([]byte, 4)
				copy(v, e.data)
				out = append(out, v...)
			default:
				out = le.AppendUint32(out, place(e.data))
			}
		}
		return le.AppendUint32(out, 0)
	}
	make1 := ascii("Rentopia")
	ifd0 := ifd([]entry{
		{tag: 0x010F, typ: 2, count: uint32(len(make1)), data: make1},
		{tag: 0x8769, typ: 4, count: 1, inline: uint32(exifAt)},
		{tag: 0x8825, typ: 4, count: 1, inline: uint32(gpsAt)},
	})
	original, offset := ascii("2026:05:04 10:02:03"), ascii("+03:00")
	exif := ifd([]entry{
		{tag: 0x9003, typ: 2, count: uint32(len(original)), data: original},
		{tag: 0x9011, typ: 2, count: uint32(len(offset)), data: offset},
	})
	lat := rationals([2]uint32{41, 1}, [2]uint32{0, 1}, [2]uint32{45, 1})
	lng := rationals([2]uint32{28, 1}, [2]uint32{58, 1}, [2]uint32{4224, 100})
	gps := ifd([]entry{
		{tag: 0x0001, typ: 2, count: 2, data: ascii("N")},
		{tag: 0x0002, typ: 5, count: 3, data: lat},
		{tag: 0x0003, typ: 2, count: 2, data: ascii("E")},
		{tag: 0x0004, typ: 5, count: 3, data: lng},
	})
	out := []byte("II")
	out = le.AppendUint16(out, 42)
	out = le.AppendUint32(out, uint32(ifd0At))
	out = append(out, ifd0...)
	out = append(out, exif...)
	out = append(out, gps...)
	return append(out, data...)
}
