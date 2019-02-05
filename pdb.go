// Package pdb contains code to read and write PalmDoc DB files.
package pdb

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

type Record struct {
	Offset   int32
	Attribs  int8
	UniqueID int32  // really an int24 but who does those?
	Data     []byte // Actual record data
}

// Pdb represents the contents of a PDB file.
type Pdb struct {
	// Database name.
	Name string
	//
	Attributes uint16
	// App-specific version of this DB file format.
	Version uint16
	// Four character code noting the DB filetype.
	Filetype string
	// Four character code noting the creating application for this databse
	Creator string
	// Database creation time.
	CreateTime time.Time
	// Time of last modification
	ModTime time.Time
	// Time of last backup
	BackupTime time.Time
	// Current modification number
	ModNum int32
	// Unique ID for this database
	UniqueIdSeed int32
	// Number of records in the database.
	RecordCount int
	// List of records in the database.
	Records []*Record
	// AppInfo is the blob of application-specific data in this
	// database. There is no predefined format for this.
	AppInfo []byte
	// SortInfo is a blob of record sorting data for this
	// database. There is no predefined format for this.
	SortInfo []byte
}

// Validate makes sure the header of the PalmDoc DB is valid
func (p *Pdb) Validate() error {
	if len(p.Creator) != 4 {
		return fmt.Errorf("Creator name must be exactly 4 characters")
	}
	if len(p.Filetype) != 4 {
		return fmt.Errorf("Filetype must be exactly 4 characters")
	}

	if err := checkTime(p.CreateTime, false); err != nil {
		return fmt.Errorf("Create time is invalid: %v", err)
	}
	if err := checkTime(p.ModTime, false); err != nil {
		return fmt.Errorf("Modification time is invalid: %v", err)
	}
	if err := checkTime(p.BackupTime, true); err != nil {
		return fmt.Errorf("Backup time is invalid: %v", err)
	}
	if len(p.Name) > 32 {
		return fmt.Errorf("Name too long")
	}

	return nil
}

func checkTime(t time.Time, zeroOK bool) error {
	if t.IsZero() {
		if zeroOK {
			return nil
		}
		return fmt.Errorf("Time value must not be zero")
	}

	lowTime := time.Unix(oldDateSecs, 0)
	highTime := time.Unix(1<<31-1, 0)

	if lowTime.After(t) || highTime.Before(t) {
		return fmt.Errorf("Time value out of range")
	}

	return nil
}

func Read(name string) (*Pdb, error) {
	fh, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	return ReadFH(fh)
}

func ReadFH(fh io.ReadSeeker) (*Pdb, error) {
	p := &Pdb{}
	err := p.readHeader(fh)
	if err != nil {
		return nil, err
	}

	// Go read the record metadata. Start with the data at offset 72.
	if err := p.readRecordMetadata(72, fh); err != nil {
		return nil, err
	}

	return p, nil
}

// readRecordMetadata parses the record list in the pdb file, filling
// out all the record structs with the offset, unique IDs, and
// attributes. It does *not* grab the actual record data; that's
// gotten later.
func (p *Pdb) readRecordMetadata(start int64, fh io.ReadSeeker) error {
	var recs int

	_, err := fh.Seek(start, io.SeekStart)
	if err != nil {
		return err
	}

	header := make([]byte, 6)
	_, err = io.ReadFull(fh, header)

	nextOffset := int64(header[0])<<24 + int64(header[1])<<16 + int64(header[2])<<8 + int64(header[3])
	recs = int(header[4])<<8 + int(header[5])

	for i := 0; i < recs; i++ {
		ri := make([]byte, 8)
		_, err := io.ReadFull(fh, ri)
		if err != nil {
			return err
		}
		rec := &Record{}
		binary.Read(bytes.NewReader(ri), binary.BigEndian, &rec.Offset)
		rec.Attribs = int8(ri[4])
		rec.UniqueID = int32(ri[5])<<16 + int32(ri[6])<<8 + int32(ri[7])
		p.Records = append(p.Records, rec)
	}

	p.RecordCount += recs

	// Do we have another batch of records? If so, go read them.
	if nextOffset > 0 {
		return p.readRecordMetadata(nextOffset, fh)
	}

	return nil
}

func (p *Pdb) Write(name string) error {
	if err := p.Validate(); err != nil {
		return err
	}
	fh, err := os.Create(name)
	if err != nil {
		return err
	}
	return p.WriteFH(fh)
}

func (p *Pdb) WriteFH(fh io.Writer) error {
	if err := p.Validate(); err != nil {
		return err
	}
	return fmt.Errorf("Unimplemented")
}

func (p *Pdb) readHeader(fh io.ReadSeeker) error {
	header := make([]byte, 72)
	_, err := io.ReadFull(fh, header)
	if err != nil {
		return err
	}

	n := string(header[0:32])
	if i := strings.Index(n, "\x00"); i != -1 {
		n = n[0 : i-1]
	}
	p.Name = n

	p.Version = uint16(header[34])<<8 + uint16(header[35])

	p.CreateTime = readTime(header[36:])
	p.ModTime = readTime(header[40:])
	p.BackupTime = readTime(header[44:])

	p.Creator = string(header[60:64])
	p.Filetype = string(header[64:68])

	return nil
}

// Base date for MOBI. Jan 1, 1904.
const oldDateSecs = -2082844800

// readTime takes the first four bytes of the passed in byte slice and
// interprets them as a mobi datestamp.
func readTime(b []byte) time.Time {
	var uInt uint32
	var sInt int32

	binary.Read(bytes.NewReader(b), binary.BigEndian, &sInt)
	// Did we get a 0? If so just return an empty time struct.
	if sInt == 0 {
		return time.Time{}
	}
	// A time > 0 (the high bit isn't set) means a time relative to unix
	// base date.
	if sInt > 0 {
		return time.Unix(int64(sInt), 0)
	}

	// A time < 0 (the high bit is set) means this is an unsigned int
	// representing a time relative to 1904. We re-parse as unsigned.
	binary.Read(bytes.NewReader(b), binary.BigEndian, &uInt)

	return time.Unix(int64(uInt)+oldDateSecs, 0)
}
