package pdb

import "testing"

func TestWrite(t *testing.T) {

}

// The sample file is a copy of Alice in Wonderland from Project Gutenberg.
const sampleFile = "pg11-images.mobi"

func TestRead(t *testing.T) {
	p, err := Read(sampleFile)
	if err != nil {
		t.Errorf("Unable to open %q: %v", sampleFile, err)
	}
	wantName := "Alices_Adven-_in_Wonderlan"
	if p.Name != wantName {
		t.Errorf("Name: got %q, want %q", p.Name, wantName)
	}
	wantCreator := "BOOK"
	if p.Creator != wantCreator {
		t.Errorf("Creator: got %q, want %q", p.Creator, wantCreator)
	}
	wantFiletype := "MOBI"
	if p.Filetype != wantFiletype {
		t.Errorf("Filetype: got %q, want %q", p.Filetype, wantFiletype)
	}
	wantVersion := uint16(0)
	if p.Version != wantVersion {
		t.Errorf("Version: got %v, want %v", p.Version, wantVersion)
	}
	wantCount := 126
	if p.RecordCount != wantCount {
		t.Errorf("RecordCount: got %v, want %v", p.RecordCount, wantCount)
	}

	wantOffset := int32(1088)
	if p.Records[0].Offset != wantOffset {
		t.Errorf("Record[0].Offset: got %v, want %v", p.Records[0].Offset, wantOffset)
	}
	wantOffset = int32(9988)
	if p.Records[1].Offset != wantOffset {
		t.Errorf("Record[1].Offset: got %v, want %v", p.Records[1].Offset, wantOffset)
	}

}
