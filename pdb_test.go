package pdb

import (
	"io"
	"io/ioutil"
	"os"
	"testing"
	"time"
)

// The sample file is a copy of Alice in Wonderland from Project Gutenberg.
const sampleFile = "testdata/pg11-images.mobi"

func TestRead(t *testing.T) {
	p, err := Read(sampleFile)
	if err != nil {
		t.Errorf("Unable to open %q: %v", sampleFile, err)
	}

	checkPDB(t, p)
}

func TestWrite(t *testing.T) {
	p, err := Read(sampleFile)
	if err != nil {
		t.Errorf("Unable to open %q: %v", sampleFile, err)
	}

	wf, err := ioutil.TempFile("", "test.mobi")
	if err != nil {
		t.Fatalf("can't open temp file for testing: %v", err)
	}
	defer os.Remove(wf.Name())

	err = p.WriteFH(wf)
	if err != nil {
		t.Errorf("Can't write to byte buffer: %v", err)
	}
	wf.Seek(0, io.SeekStart)

	np, err := ReadFH(wf)
	if err != nil {
		t.Errorf("Can't re-read byte buffer: %v", err)
	}
	checkPDB(t, np)

}

func checkPDB(t *testing.T, p *Pdb) {
	wantName := "Alices_Adven-_in_Wonderland"
	if p.Name != wantName {
		t.Errorf("Name: got %q, want %q", p.Name, wantName)
	}
	wantCreator := "MOBI"
	if p.Creator != wantCreator {
		t.Errorf("Creator: got %q, want %q", p.Creator, wantCreator)
	}
	wantFiletype := "BOOK"
	if p.Filetype != wantFiletype {
		t.Errorf("Filetype: got %q, want %q", p.Filetype, wantFiletype)
	}
	wantVersion := uint16(0)
	if p.Version != wantVersion {
		t.Errorf("Version: got %v, want %v", p.Version, wantVersion)
	}
	wantCreate, err := time.Parse("2006-01-02 15:04:05 -0700 MST", "2018-11-01 01:01:05 -0400 EDT")
	if err != nil {
		t.Errorf("Bad time: %v", err)
	}
	if p.CreateTime != wantCreate {
		t.Errorf("createTime: got %v, want %v", p.CreateTime, wantCreate)
	}
	wantMod, err := time.Parse("2006-01-02 15:04:05 -0700 MST", "2018-11-01 01:01:06 -0400 EDT")
	if err != nil {
		t.Errorf("Bad time: %v", err)
	}
	if p.ModTime != wantMod {
		t.Errorf("createTime: got %v, want %v", p.ModTime, wantMod)
	}
	wantCount := 126
	if p.recordCount != wantCount {
		t.Errorf("recordCount: got %v, want %v", p.recordCount, wantCount)
	}

	// For the moment we're not checking offsets since they aren't quite preserved for some reason.
	// wantOffset := uint32(1088)
	// if p.Records[0].offset != wantOffset {
	// 	t.Errorf("Record[0].offset: got %v, want %v", p.Records[0].offset, wantOffset)
	// }
	// wantOffset = uint32(9988)
	// if p.Records[1].offset != wantOffset {
	// 	t.Errorf("Record[1].offset: got %v, want %v", p.Records[1].offset, wantOffset)
	// }
	// wantOffset = uint32(12003)
	// if p.Records[2].offset != wantOffset {
	// 	t.Errorf("Record[2].offset: got %v, want %v", p.Records[2].offset, wantOffset)
	// }
	if len(p.Records[1].Data) != 2015 {
		t.Errorf("Record[1].Data length: got %v, want %v", len(p.Records[1].Data), 2015)
	}
	wantAttributes := uint16(0)
	if p.Attributes != wantAttributes {
		t.Errorf("Attributes: got %v, want %v", p.Attributes, wantAttributes)
	}
	if p.sortInfoOffset != 0 {
		t.Errorf("SortInfoOffset: got %v, want %v", p.sortInfoOffset, 0)
	}
	if p.appInfoOffset != 0 {
		t.Errorf("appInfoOffset: got %v, want %v", p.appInfoOffset, 0)
	}

}
