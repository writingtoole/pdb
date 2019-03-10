package lz77

import (
	"bytes"
	"io/ioutil"
	"math/rand"
	"strings"
	"testing"
)

func randomJunk(seed int64, l int) []byte {
	r := rand.New(rand.NewSource(seed))
	ret := make([]byte, l)
	r.Read(ret)
	return ret
}

func findDiff(l, r []byte) (bool, int) {
	m := len(l)
	if len(r) < len(l) {
		m = len(r)
	}
	for i := 0; i < m; i++ {
		if l[i] != r[i] {
			return false, i
		}
	}
	if len(l) != len(r) {
		return false, m
	}
	return true, 0
}

func TestBinary(t *testing.T) {
	tests := []struct {
		name     string
		contents []byte
	}{
		{
			"first",
			[]byte{12, 38, 194, 107, 170, 117, 157, 168, 106, 199, 122, 69, 32, 197, 78, 37, 252, 34, 221, 92},
		},
	}

	for _, test := range tests {
		c, _ := Compress(test.contents)
		got, _ := Decompress(c)
		if !bytes.Equal(test.contents, got) {
			t.Errorf("%v:\n%v\n%v\n%v", test.name, c, test.contents, got)
		}
	}
}

func TestBinaryBlobs(t *testing.T) {
	for l := 1024; l < 1024*100; l += 1024 {
		want := randomJunk(int64(l), l)
		c, _ := Compress(want)
		got, err := Decompress(c)
		if err != nil {
			t.Errorf("error decompressing binary blob %v: %v", l, err)
			continue
		}
		if eq, fd := findDiff(got, want); !eq {
			t.Errorf("random binary %v (%v/%v/%v) mismatch\n%v\n%v\n%v", l, len(want), len(got), fd, c[0:15], got[0:15], want[0:15])
			break
		}
	}
}

func TestDecompress(t *testing.T) {
	tests := []struct {
		name       string
		compressed []byte
		want       []byte
	}{
		{
			name:       "Empty",
			compressed: []byte{},
			want:       []byte{},
		},
		{
			name:       "Literal bytes",
			compressed: []byte{0x40, 0x50, 0x60},
			want:       []byte{0x40, 0x50, 0x60},
		},
		{
			name:       "Space encoded",
			compressed: []byte{0x80 ^ 0x45},
			want:       []byte{' ', 0x45},
		},
		{
			name:       "Literal chunk",
			compressed: []byte{0x05, 0x01, 0x02, 0x03, 0x04, 0x05},
			want:       []byte{0x01, 0x02, 0x03, 0x04, 0x05},
		},
		{
			name:       "Previous run",
			compressed: []byte{'a', 'b', 'c', 'd', 'e', 'f', 0x80, 0x05<<3 | 0x01},
			want:       []byte{'a', 'b', 'c', 'd', 'e', 'f', 'b', 'c', 'd', 'e'},
		},
		{
			name:       "Overlap run",
			compressed: []byte{'a', 'b', 'c', 'd', 0x80, 0x02<<3 | 0x01},
			want:       []byte{'a', 'b', 'c', 'd', 'c', 'd', 'c', 'd'},
		},
		{
			name:       "Repeat last byte",
			compressed: []byte{'a', 'b', 0x80, 0x01<<3 | 0x06},
			want:       []byte{'a', 'b', 'b', 'b', 'b', 'b', 'b', 'b', 'b', 'b', 'b'},
		},
	}

	for _, test := range tests {
		got, _ := Decompress(test.compressed)
		if !bytes.Equal(test.want, got) {
			t.Errorf("Decompress(%v):\ngot  %v\nwant %v", test.name, got, test.want)
		}
	}
}

func TestRoundTrip(t *testing.T) {
	tests := []struct {
		name     string
		contents []byte
	}{
		{
			"empty",
			[]byte(""),
		},
		{
			"basic",
			[]byte("this is basic text"),
		},
		{
			"reps",
			[]byte("rep rep rep rep rep rep rep rep rep rep rep rep"),
		},
		{
			"mondo reps",
			[]byte(strings.Repeat("stringy stuff", 100)),
		},
	}
	for _, test := range tests {
		compressed, err := Compress(test.contents)
		if err != nil {
			t.Errorf("Test %q got compression error: %v", test.name, err)
			continue
		}
		got, err := Decompress(compressed)
		if !bytes.Equal(got, test.contents) {
			t.Errorf("Test %q mismatch:\n%v\n%v\n%v\n%v\n%v\n%v\n", test.name, compressed, string(compressed), got, test.contents, string(got), string(test.contents))
		}
	}

}

func TestByteLiteral(t *testing.T) {
	tests := []struct {
		in   byte
		want []byte
	}{
		{0, []byte{0}},
		{1, []byte{1, 1}},
		{0x09, []byte{0x09}},
		{0xf4, []byte{1, 0xf4}},
	}

	for _, test := range tests {
		got := byteLiteral(test.in)
		if !bytes.Equal(test.want, got) {
			t.Errorf("Encoding %v got %v want %v", test.in, got, test.want)
		}
	}
}

func TestFindRun(t *testing.T) {
	tests := []struct {
		name       string
		data       string
		seen       string
		wantOffset int
		wantLen    int
	}{
		{
			name:       "no match",
			data:       "abcde",
			seen:       "12345",
			wantOffset: -1,
			wantLen:    -1,
		},
		{
			name:       "too short",
			data:       "ab",
			seen:       "abcde",
			wantOffset: -1,
			wantLen:    -1,
		},
		{
			name:       "simple run",
			data:       "1234",
			seen:       "abc1234567",
			wantOffset: 7,
			wantLen:    4,
		},
		{
			name:       "multi run",
			data:       "12345",
			seen:       "1234abc12345",
			wantOffset: 5,
			wantLen:    5,
		},
		{
			name:       "multi multi run",
			data:       "12345",
			seen:       "1234abc1234512345",
			wantOffset: 10,
			wantLen:    5,
		},
	}

	for _, test := range tests {
		gotLen, gotOffset := findRun([]byte(test.data), []byte(test.seen))
		if gotLen != test.wantLen || gotOffset != test.wantOffset {
			t.Errorf("findRun(%v): %q/%q, got %v/%v, want %v/%v", test.name, test.data, test.seen, gotOffset, gotLen, test.wantOffset, test.wantLen)
		}
	}
}

func TestFileRoundTrip(t *testing.T) {
	files := []string{
		"lz77_test.go",
		"lz77.go",
		"testdata/ioutil.html",
	}

	for _, file := range files {
		contents, err := ioutil.ReadFile(file)
		if err != nil {
			t.Errorf("error reading %v: %v", file, err)
			continue
		}
		c, _ := Compress(contents)
		d, _ := Decompress(c)
		if eq, fd := findDiff(contents, d); !eq {
			t.Errorf("Mismatch round-tripping %v at %v", file, fd)
		}
	}

}
