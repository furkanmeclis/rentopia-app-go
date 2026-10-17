package usecase

import (
	"encoding/binary"
	"testing"
)

func TestWebPDimensions(t *testing.T) {
	cases := []struct {
		name          string
		chunk         string
		payload       []byte
		width, height int
	}{
		{"lossy", "VP8 ", []byte{0, 0, 0, 0x9d, 0x01, 0x2a, 0x23, 0x01, 0x34, 0x02}, 291, 564},
		{"lossless", "VP8L", []byte{0x2f, 0x22, 0xc1, 0x8c, 0}, 291, 564},
		{"extended", "VP8X", []byte{0, 0, 0, 0, 0x22, 0x01, 0, 0x33, 0x02, 0}, 291, 564},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			data := []byte("RIFF")
			length := 4 + 8 + len(tt.payload) + (len(tt.payload) & 1)
			data = binary.LittleEndian.AppendUint32(data, uint32(length))
			data = append(data, []byte("WEBP"+tt.chunk)...)
			data = binary.LittleEndian.AppendUint32(data, uint32(len(tt.payload)))
			data = append(data, tt.payload...)
			if len(tt.payload)&1 != 0 {
				data = append(data, 0)
			}
			got, err := mediaDimensions(data, "image/webp")
			if err != nil || got.Width != tt.width || got.Height != tt.height {
				t.Fatalf("got %+v, %v", got, err)
			}
			// Every truncated prefix must fail without reading beyond its buffer.
			for i := range len(data) {
				if _, err := mediaDimensions(data[:i], "image/webp"); err == nil {
					t.Fatalf("accepted truncated header %d", i)
				}
			}
		})
	}
}
