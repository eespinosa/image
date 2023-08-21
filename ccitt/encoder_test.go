package ccitt

import (
	"bytes"
	"fmt"
	"image"
	"io"
	"os"
	"testing"
)

func TestEncode(t *testing.T) {
	testCases := []struct {
		fileName string
		w, h     int
		aligned  bool
	}{
		{"testdata/bw-gopher.ccitt_group4", 153, 55, false},
		{"testdata/bw-gopher-aligned.ccitt_group4", 153, 55, true},
	}

	for _, tc := range testCases {

		width, height := tc.w, tc.h

		img, err := decodePNG("testdata/bw-gopher.png")
		if err != nil {
			t.Fatalf("decodePNG: %v", err)
		}
		gray, ok := img.(*image.Gray)
		if !ok {
			t.Fatalf("decodePNG: got %T, want *image.Gray", img)
		}

		bounds := gray.Bounds()
		bwBytes := []byte(nil)

		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			rowPix := gray.Pix[(y-bounds.Min.Y)*gray.Stride:]
			rowPix = rowPix[:width]
			var rowBytes []byte

			// change from grayscale to 0x00 and 0xFF
			for _, pixel := range rowPix {
				if (pixel & 0x80) != 0x00 {
					rowBytes = append(rowBytes, 0x00)
				} else {
					rowBytes = append(rowBytes, 0xFF)
				}
			}

			fmt.Printf("row%02d: %02X\n", y, rowBytes)

			bwBytes = append(bwBytes, rowBytes...)
		}

		gotBytes, err := EncodeGroup4(width, height, bwBytes, tc.aligned)
		if err != nil {
			t.Fatalf("failed to encode: %v", err)
		}

		f, err := os.Open(tc.fileName)
		if err != nil {
			t.Fatalf("failed to open file: %v", err)
		}
		wantBytes, err := io.ReadAll(f)
		if err != nil {
			t.Fatalf("failed to read file: %v", err)
		}
		f.Close()

		if !bytes.Equal(gotBytes, wantBytes) {
			t.Fatalf("encoder output does not match test file output")
		}
	}
}
