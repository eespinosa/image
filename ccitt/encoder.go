package ccitt

import (
	"bytes"
)

var eofbCode = bitString{0x1001, 24}

func EncodeGroup4(width, height int, pixels []byte, aligned bool) ([]byte, error) {
	var bb bytes.Buffer
	w := &bitWriter{w: &bb, order: MSB}

	prev := make([]byte, width)

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
		return start
	}
	currentColor := byte(0x00)
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
	a0Color := byte(0x00)
	for a0 < len(curr) {
		// 1: Find a1 (first changing element to the right of a0)
		a1 := findNextChangingElement(curr, a0)
		// 2: Find b1 (first changing element right of a0 and opposite color to a1)
		b1 := findNextChangingElement(prev, a0)
		if b1 != len(prev) && prev[b1] != ^a0Color {
			b1 = findNextChangingElement(prev, b1)
		}
		// 3: Find b2 (first changing element to the right of b1)
		b2 := findNextChangingElement(prev, b1)

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
			w.writeCode(modeEncodeTable[modeH])
			// Find a2 (next changing element to the right of a1)
			a2 := findNextChangingElement(curr, a1)
			// Horizontal mode coding (encode run lengths a0a1 and a1a2 according to tables)
			a0a1 := a1 - a0
			if a0 == -1 {
				a0a1 -= 1
			}
			err := encodeModeH(a0a1, a0Color, w)
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
	if color == 0x00 {
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
	if r != 0 {
		err := w.writeCode(encTableSmall[r])
		if err != nil {
			return err
		}
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
