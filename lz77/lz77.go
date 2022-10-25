// Package lz77 implements lz77 compression and decompression, as used
// by PalmDoc DB files.
package lz77

import (
	"bytes"
	"fmt"
	"log"
)

// Compress takes a slice of bytes and returns a compressed version of
// it. Compression is done in 4096 byte blocks for historical reasons.
func Compress(data []byte) ([]byte, error) {
	// Is our input less than one block? If so just compress it and be
	// done. No need for chunking.
	if len(data) <= 4096 {
		return compressBlock(data), nil
	}

	ret := make([]byte, 0, len(data))
	for start := 0; start < len(data); start += 4096 {
		end := start + 4096
		if end > len(data) {
			end = len(data)
		}
		c := compressBlock(data[start:end])
		ret = append(ret, c...)
	}

	return ret, nil
}

// byteLiteral takes a byte and returns the lz77 encoded version of it.
func byteLiteral(b byte) []byte {
	switch {
	case b == 0:
		return []byte{b}
	case b >= 0x09 && b <= 0x7f:
		return []byte{b}
	default:
		return []byte{1, b}
	}
}

// compressBlock compresses a single up-to-4096 byte block of the input.
func compressBlock(data []byte) []byte {
	// Preallocate the output slice on the optimistic assumption that
	// the output won't be bigger than the input.
	ret := make([]byte, 0, len(data))
	for i := 0; i < len(data); i++ {
		// Last byte in the input? Encode it and be done.
		if i == len(data)-1 {
			ret = append(ret, byteLiteral(data[i])...)
			continue
		}

		// Have we seen a run already? If so then encode it.
		l, offset := findRun(data[i:], data[0:i])
		if l >= 3 {
			// 10 bytes is our maximum run length.
			if l > 10 {
				l = 10
			}
			word := uint16(offset<<3+(l-3)) | 0x8000
			ret = append(ret, byte(word>>8), byte(word&0xff))

			i += (l - 1)
			continue
		}

		// space + printable? Add in the special byte and be done.
		if data[i] == ' ' && (data[i+1] >= 0x40 && data[i+1] <= 0x7f) {
			ret = append(ret, 0x80^data[i+1])
			i++
			continue
		}

		// A literal character? Then just pass it on to the output stream.
		if (data[i] >= 0x09 && data[i] <= 0x7f) || data[i] == 0 {
			ret = append(ret, data[i])
			continue
		}

		// Not a literal. In that case we need to blob a range of bytes --
		// send out a chunk as big as we can.
		max := len(data) - i
		if max > 8 {
			max = 8
		}
		ret = append(ret, byte(max))
		ret = append(ret, data[i:i+max]...)
		i += (max - 1)
		continue
	}

	return ret
}

// findRun looks back in the data we've already compressed to see if
// we can find a chunk that matches the data that's left to be compressed.
func findRun(data []byte, seen []byte) (int, int) {
	// If we don't even have 3 bytes left then we can't have a run.
	if len(data) < 3 {
		return -1, -1
	}
	idx := -1
	l := -1

	// we can only look back 1024 bytes, since the offset has to be
	// encoded in 11 bits.
	if len(seen) > 1024 {
		e := len(seen)
		b := e - 1024
		seen = seen[b:e]
	}
	for max := 3; max < 11 && max <= len(data); max++ {
		offset := bytes.Index(seen, data[0:max])
		if offset == -1 {
			break
		}
		idx = len(seen) - offset
		l = max
	}

	return l, idx
}

// Decompress decompresses a compressed block of data.
func Decompress(data []byte) ([]byte, error) {
	// Start off assuming that decompressing a buffer makes the result
	// larger. This is mostly but not always true.
	ret := make([]byte, 0, len(data)*2)
	for o := 0; o < len(data); o++ {
		b := data[o]
		switch {
		case b == 0:
			ret = append(ret, b)
		case (b >= 1 && b <= 8):
			if o+int(b)+1 > len(data) {
				return nil, fmt.Errorf("copy from past end of block: %v/%v", len(data), o+int(b)+1)
			}
			d := data[o+1 : o+int(b)+1]
			ret = append(ret, d...)
			o += int(b)
		case (b >= 0x09 && b <= 0x7f):
			ret = append(ret, b)
		case b >= 0x80 && b <= 0xbf:
			o++
			m := int(b)<<8 + int(data[o])
			dist := (m & 0x3fff) >> 3
			l := m&0x07 + 3
			if dist > len(ret) {
				return nil, fmt.Errorf("dist %v, len %v but len(ret) only %v (%x)", dist, l, len(ret), m)
			}
			if dist < 1 {
				log.Printf("dist %v is less than 1", dist)
				dist = 1
			}
			sl := len(ret)
			for i := 0; i < l; i++ {
				idx := (len(ret) - dist)
				if idx < 0 || idx >= len(ret) {
					log.Printf("Out of range; started %v, off %v, len %v, curidx %v, curlen %v", sl, dist, l, idx, len(ret))
				}
				sb := ret[idx]
				ret = append(ret, sb)
			}
		case b >= 0xc0:
			ret = append(ret, ' ')
			ret = append(ret, b^0x80)
		default:
			return nil, fmt.Errorf("unknown byte %v", b)
		}

	}

	return ret, nil
}
