package ccitt

import (
	"bytes"
	"io"
)

var eofbCode = bitString{0x1001, 24}

type writer struct {
	bw bitWriter

	width int

	curr []byte
	prev []byte

	// currPosition keeps track of the last position
	// written to curr. When it reaches width, we can encode
	// the row.
	currPosition int

	rowsEncoded int
}

const (
	colorWhite = byte(0xFF)
	colorBlack = byte(0x00)
)

func (w *writer) Write(p []byte) (int, error) {
	// for each byte in p write byte to curr[currPosition]
	// until currPosition == width, then encode row
	writtenBytes := 0
	n := 0
	for _, b := range p {
		// Convert from gray to black (0xFF) or white (0x00)
		pixel := colorBlack
		if (b & 0x80) != 0x00 {
			pixel = colorWhite
		}
		w.curr[w.currPosition] = pixel
		n += 1
		w.currPosition += 1

		if w.currPosition == w.width {
			err := encodeRow(w.curr, w.prev, &w.bw)
			if err != nil {
				return writtenBytes, err
			}
			w.prev = w.curr[:]
			w.curr = make([]byte, w.width)
			// Update actual bytes written to wrapped writer
			writtenBytes += n
			w.currPosition = 0
			n = 0
			w.rowsEncoded += 1
		}
	}

	return writtenBytes, nil
}

func (w *writer) Flush() error {
	err := w.bw.flushBits()
	return err
}

func (w *writer) Close() error {
	// output end of facsimile block code
	err := w.bw.writeCode(eofbCode)
	if err != nil {
		return err
	}

	return w.bw.close()
}

func NewGroup4Encoder(w io.Writer, width int) *writer {
	return &writer{
		bw:           bitWriter{w: w, order: MSB},
		width:        width,
		prev:         nil,
		curr:         make([]byte, width),
		currPosition: 0,
		rowsEncoded:  0,
	}
}

func EncodeGroup4(width, height int, pixels []byte, aligned bool) ([]byte, error) {
	var bb bytes.Buffer
	w := &bitWriter{w: &bb, order: MSB}

	var prev []byte

	for y := 0; y < height; y++ {
		row := pixels[y*width:]
		row = row[:width]

		err := encodeRow(row, prev, w)
		if err != nil {
			return nil, err
		}
		prev = row[:]
		if aligned {
			err = w.alignToByteBoundary()
			if err != nil {
				return nil, err
			}
		}
	}

	// output end of facsimile block code
	err := w.writeCode(eofbCode)
	if err != nil {
		return nil, err
	}

	err = w.close()
	if err != nil {
		return nil, err
	}
	return bb.Bytes(), nil
}

func findNextChangingElement(row []byte, start int) int {
	if start >= len(row) {
		return len(row)
	}
	currentColor := colorWhite
	if start == -1 {
		start = 0
	} else {
		currentColor = row[start]
	}

	// Loop invariant: row[i] is the same color as currentColor
	var i int
	for i = start; (i < len(row)) && (row[i] == currentColor); i++ {
	}
	// Now either i == len(row), and is therefore an imaginary changing element,
	// or i < len(row), and is therefore an actual changing element.
	return i
}

// Given the current row and the previous (reference) row, output the coded current row.
func encodeRow(curr, prev []byte, w *bitWriter) error {
	// 0: Set a0 to -1 with color white
	a0 := -1
	a0Color := colorWhite
	for a0 < len(curr) {
		// 1: Find a1 (first changing element to the right of a0)
		a1 := findNextChangingElement(curr, a0)
		// 2: Find b1 (first changing element right of a0 and opposite color to a0)
		// The first row is a special case
		var b1, b2 int
		if len(prev) == 0 {
			b1 = len(curr)
			b2 = len(curr)
		} else {
			b1 = findNextChangingElement(prev, a0)
			if b1 != len(prev) && prev[b1] != ^a0Color {
				b1 = findNextChangingElement(prev, b1)
			}
			// 3: Find b2 (first changing element to the right of b1)
			b2 = findNextChangingElement(prev, b1)
		}

		// If b2 is to the left of a1
		if b2 < a1 {
			// Pass mode coding (output 0001)
			err := encodeModePass(w)
			if err != nil {
				return err
			}
			// Set a0 to under b2
			a0 = b2
			// b2 is to the left of a1, which means a0Color stays the same
			// Goto 1
			continue
		}
		// If horizontal distance between a1 and b1 is 3 or less
		hDist := a1 - b1
		if (hDist < 4) && (hDist > -4) {
			err := encodeModeV(hDist, w)
			if err != nil {
				return err
			}
			// Set a0 to a1
			a0 = a1
			a0Color = ^a0Color // a1 is a "changing element" so must be the opposite color
			// Else
		} else {
			err := w.writeCode(modeEncodeTable[modeH])
			if err != nil {
				return err
			}
			// Find a2 (next changing element to the right of a1)
			a2 := findNextChangingElement(curr, a1)
			// Horizontal mode coding (encode run lengths a0a1 and a1a2 according to tables)
			a0a1 := a1 - a0
			if a0 == -1 {
				a0a1 -= 1
			}
			err = encodeModeH(a0a1, a0Color, w)
			if err != nil {
				return err
			}

			a1a2 := a2 - a1
			err = encodeModeH(a1a2, ^a0Color, w)
			if err != nil {
				return err
			}
			// Set a0 to a2
			a0 = a2
			// a2 is a changing element after a1, which is a changing element after a0,
			// so a2 is the same color as a0, so there is nothing to update
		}
		// If end of line
		if a0 == len(curr) {
			// Return
			break
		}
		// Goto 1
	}
	return nil
}

func encodeModeH(runLength int, color byte, w *bitWriter) error {
	encTableSmall := blackEncodeTable2[:]
	encTableBig := blackEncodeTable3[:]
	if color == colorWhite {
		encTableSmall = whiteEncodeTable2[:]
		encTableBig = whiteEncodeTable3[:]
	}

	q := runLength / 64
	if q > 0 {
		err := w.writeCode(encTableBig[q-1])
		if err != nil {
			return err
		}
	}
	r := runLength - (q * 64)
	// Write the code word for r even if it is 0
	err := w.writeCode(encTableSmall[r])
	if err != nil {
		return err
	}

	return nil
}

func encodeModeV(distance int, w *bitWriter) error {
	// Vertical mode coding (output code word for one of the 6 cases)
	var err error
	switch distance {
	case -1:
		err = w.writeCode(modeEncodeTable[modeVL1])
	case -2:
		err = w.writeCode(modeEncodeTable[modeVL2])
	case -3:
		err = w.writeCode(modeEncodeTable[modeVL3])
	case 1:
		err = w.writeCode(modeEncodeTable[modeVR1])
	case 2:
		err = w.writeCode(modeEncodeTable[modeVR2])
	case 3:
		err = w.writeCode(modeEncodeTable[modeVR3])
	default:
		err = w.writeCode(modeEncodeTable[modeV0])
	}

	return err
}

func encodeModePass(w *bitWriter) error {
	err := w.writeCode(modeEncodeTable[modePass])
	return err
}
