// Package lz77 implements lz77 compression and decompression, as used
// by PalmDoc DB files.
package lz77

import (
	"bytes"
	"log"
)

// Compress
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

// byteLiteral takes a byte and returns the compressed version of it.
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
			//			//log.Printf("writing one byte literal %v", data[i])
			ret = append(ret, byteLiteral(data[i])...)
			continue
		}

		// Have we seen a run already?
		l, offset := findRun(data[i:], data[0:i])
		if l >= 3 {
			// 10 bytes is our max.
			if l > 10 {
				l = 10
			}
			word := uint16(offset<<3+(l-3)) | 0x8000
			ret = append(ret, byte(word>>8), byte(word&0xff))
			//			d := data[i-offset : i-offset+l]
			//log.Printf("%v writing %v byte run %v (%x %x %x off: %v l: %v)", i, l, d, byte(word>>8), byte(word&0xff), word, offset, l)

			i += (l - 1)
			continue
		}

		// space + printable? Add in the special byte and be done.
		if data[i] == ' ' && (data[i+1] >= 0x40 && data[i+1] <= 0x7f) {
			ret = append(ret, 0x80^data[i+1])
			//log.Printf("%v writing space pair %v", i, data[i+1])
			i++
			continue
		}

		// A literal character? Then just pass it on to the output stream.
		if (data[i] >= 0x09 && data[i] <= 0x7f) || data[i] == 0 {
			ret = append(ret, data[i])
			//log.Printf("%v writing character literal %v", i, data[i])
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
		//log.Printf("%v writing %v byte set %v", i, max, data[i:i+max])
		i += (max - 1)
		continue
	}

	return ret
}

func findRun(data []byte, seen []byte) (int, int) {
	// If we don't even have 3 bytes left then we can't have a run.
	if len(data) < 3 {
		return -1, -1
	}
	idx := -1
	l := -1

	// we can only look back 1024 bytes
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

func Decompress(data []byte) ([]byte, error) {
	// Start off assuming that decompressing a buffer makes the result larger.
	ret := make([]byte, 0, len(data)*2)
	for o := 0; o < len(data); o++ {
		//		curset := len(ret)
		b := data[o]
		switch {
		case b == 0:
			//log.Printf("%v reading character literal %v", curset, b)
			ret = append(ret, b)
		case (b >= 1 && b <= 8):
			d := data[o+1 : o+int(b)+1]
			ret = append(ret, d...)
			//log.Printf("%v reading %v byte set %v", curset, b, d)
			o += int(b)
		case (b >= 0x09 && b <= 0x7f):
			//log.Printf("%v reading character literal %v", curset, b)
			ret = append(ret, b)
		case b >= 0x80 && b <= 0xbf:
			o++
			m := int(b)<<8 + int(data[o])
			dist := (m & 0x3fff) >> 3
			l := m&0x07 + 3
			if dist > len(ret) {
				log.Fatalf("dist %v, len %v but len(ret) only %v (%x)", dist, l, len(ret), m)
			}
			d := ret[len(ret)-dist : len(ret)-dist+l]
			ret = append(ret, d...)
			//log.Printf("%v reading %v byte run %v", curset, l, d)
		case b >= 0xc0:
			ret = append(ret, ' ')
			ret = append(ret, b^0x80)
			//log.Printf("%v reading space pair %v", curset, b^0x80)
		default:
			log.Fatalf("unknown byte %v", b)
		}

	}

	return ret, nil
}
