// Package pdb contains code to read and write PalmDoc DB files.
package pdb

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"
)

// Record holds the contents of a single record in the PDB file.
type Record struct {
	offset uint32
	end    uint32
	// Record attributes
	Attribs int8
	// Unique ID for the record. Often just the record number. Pay no
	// attention to the top byte, this is a 24 bit integer.
	UniqueID uint32
	// Contents of the record.
	Data []byte
}

// Pdb represents the contents of a PDB file.
type Pdb struct {
	// Database name.
	Name string
	// Attribute bits
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
	ModNum uint32
	// Unique ID for this database
	UniqueIdSeed uint32
	// Number of records in the database.
	recordCount int
	// List of records in the database.
	Records []*Record
	// Offset to the start of the appinfo block
	appInfoOffset uint32
	// Offset to the end of the appinfo block
	appInfoEnd uint32
	// AppInfo is the blob of application-specific data in this
	// database. There is no predefined format for this.
	AppInfo []byte
	// Offset of the start of the sortinfo block
	sortInfoOffset uint32
	// Offset of the end of the sortinfo block
	sortInfoEnd uint32
	// SortInfo is a blob of record sorting data for this
	// database. There is no predefined format for this.
	SortInfo []byte
	// Total size of the file.
	totalSize int64
}

// The on-disk header format
type header struct {
	Name           [32]byte
	Attr           uint16
	Version        uint16
	Create         uint32
	Modified       uint32
	Backup         uint32
	ModNum         uint32
	AppInfoOffset  uint32
	SortInfoOffset uint32
	Filetype       [4]byte
	Creator        [4]byte
	UniqueIdSeed   uint32
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
	ui := make(map[uint32]int)
	for i, r := range p.Records {
		if r.UniqueID > 0xffffff {
			return fmt.Errorf("Record %v has more than 24 bits of unique id: %v", i, r.UniqueID)
		}
		if or, ok := ui[r.UniqueID]; ok {
			return fmt.Errorf("Record %v and %v have the same UniqueID of %v", i, or, r.UniqueID)
		}
		ui[r.UniqueID] = i
	}

	return nil
}

// checkTime makes sure the passed-in time is valid for PDB files. If
// zeroOK is set then the time may be 0, otherwise not.
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

// Read the named file, parse it, and return a parsed structure.
func Read(name string) (*Pdb, error) {
	fh, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	return ReadFH(fh)
}

// ReadFH reads a PDB file open on seekable io handle.
func ReadFH(fh io.ReadSeeker) (*Pdb, error) {
	p := &Pdb{}
	size, err := fh.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, err
	}
	fh.Seek(0, io.SeekStart)
	p.totalSize = size
	err = p.readHeader(fh)
	if err != nil {
		return nil, err
	}

	// Go read the record metadata. Start with the data at offset 72.
	if err := p.readRecordMetadata(72, fh); err != nil {
		return nil, err
	}

	// Now that we've read the headers and all the record offsets we can
	// figure out how big each record is.
	if err := p.updateRecordSizes(); err != nil {
		return nil, err
	}

	if err := p.readRecordData(fh); err != nil {
		return nil, err
	}

	return p, nil
}

// readRecordData actually reads the data for the records from the file.
func (p *Pdb) readRecordData(fh io.ReadSeeker) error {
	for _, r := range p.Records {
		_, err := fh.Seek(int64(r.offset), io.SeekStart)
		if err != nil {
			return err
		}
		l := int(r.end - r.offset + 1)
		b := make([]byte, l)
		n, err := io.ReadFull(fh, b)
		if err != nil {
			return err
		}
		if n != l {
			return fmt.Errorf("Wanted to read %v bytes but read %v", l, n)
		}
		r.Data = b
	}
	return nil
}

// updateRecordSizes figures out how big each record actually is. PDB
// records have a start but no length or end, so we have to kind of
// figure this out. We do it by assuming there are no overlapping
// records, sorting the starts and working from that list.
func (p *Pdb) updateRecordSizes() error {
	// There's no guarantee that the records are listed in order in the
	// file, so we need to make a copy of the record slice so we can
	// sort it.
	r := make([]*Record, len(p.Records))
	copy(r, p.Records)
	sort.Slice(r, func(i, j int) bool { return r[i].offset < r[j].offset })

	// If we have either an appinfo or sortinfo block then tenatively
	// set their ends to EOF. This is probably wrong, but we'll fix it
	// up as we scan through the records.
	if p.appInfoOffset > 0 {
		p.appInfoEnd = uint32(p.totalSize - 1)
	}
	if p.sortInfoOffset > 0 {
		p.sortInfoEnd = uint32(p.totalSize - 1)
	}
	// The end of each record is the start of the next one
	for i, rec := range r {
		if i < len(r)-1 {
			rec.end = r[i+1].offset - 1
		} else {
			// The last record ends at EOF
			rec.end = uint32(p.totalSize - 1)
		}
		// Is the sort or app area in the "middle" of the record? If so fix it so it isn't.
		if rec.offset < p.sortInfoOffset && rec.end > p.sortInfoOffset {
			rec.end = p.sortInfoOffset - 1
		}
		if rec.offset < p.appInfoOffset && rec.end > p.appInfoOffset {
			rec.end = p.appInfoOffset - 1
		}

		// If we have appinfo, then see where the
		if rec.offset < p.appInfoEnd && rec.offset > p.appInfoOffset {
			p.appInfoEnd = rec.offset - 1
		}

		if rec.offset < p.sortInfoEnd && rec.offset > p.sortInfoOffset {
			p.sortInfoEnd = rec.offset - 1
		}
	}

	if p.appInfoOffset > p.sortInfoOffset && p.appInfoOffset < p.sortInfoEnd {
		p.sortInfoEnd = p.appInfoOffset - 1
	}

	if p.sortInfoOffset > p.appInfoOffset && p.sortInfoOffset < p.appInfoEnd {
		p.appInfoEnd = p.sortInfoOffset - 1
	}

	return nil
}

// readRecordMetadata parses the record list in the pdb file, filling
// out all the record structs with the offset, unique IDs, and
// attributes. It does *not* grab the actual record data; that's
// gotten later.
func (p *Pdb) readRecordMetadata(start uint32, fh io.ReadSeeker) error {
	var recs int

	_, err := fh.Seek(int64(start), io.SeekStart)
	if err != nil {
		return err
	}

	header := make([]byte, 6)
	_, err = io.ReadFull(fh, header)

	nextOffset := binary.BigEndian.Uint32(header[0:])
	recs = int(binary.BigEndian.Uint16(header[4:]))

	for i := 0; i < recs; i++ {
		ri := make([]byte, 8)
		_, err := io.ReadFull(fh, ri)
		if err != nil {
			return err
		}
		rec := &Record{}
		rec.offset = binary.BigEndian.Uint32(ri)
		rec.Attribs = int8(ri[4])
		// The uniqueID is a three-byte word, so we have to unpack it by hand.
		rec.UniqueID = uint32(ri[5])<<16 + uint32(ri[6])<<8 + uint32(ri[7])
		p.Records = append(p.Records, rec)
	}

	p.recordCount += recs

	// Do we have another batch of records? If so, go read them.
	if nextOffset > 0 {
		return p.readRecordMetadata(nextOffset, fh)
	}

	return nil
}

// Write serializes the PDB to the named file. If the file exists it will be overwritten.
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

// WriteFH writes the passed in PDB file to the filehandle. Any
// existing contents of the file will be overwritten.
func (p *Pdb) WriteFH(fh io.Writer) error {
	// Make sure we have valid times in the header.
	t := time.Now()
	if p.CreateTime.IsZero() {
		p.CreateTime = t
	}
	if p.ModTime.IsZero() {
		p.ModTime = t
	}

	if err := p.Validate(); err != nil {
		return err
	}
	if err := p.fixUpMetadata(); err != nil {
		return err
	}

	if err := p.writeHeader(fh); err != nil {
		return err
	}
	if err := p.writeRecordMetadata(fh); err != nil {
		return err
	}
	if err := p.writeMetadata(fh); err != nil {
		return err
	}
	if err := p.writeRecords(fh); err != nil {
		return err
	}

	return nil
}

// fixUpMetadata goes through the records and sort/info blobs,
// calculating the offset information so we can write it out later. We
// have to do this because the record headers come before the record
// bodies, but need to have the offset data in them.
func (p *Pdb) fixUpMetadata() error {
	// How many records do we have? If it's more than 64K then we need
	// to chunk them, which is annoying. For now we won't allow that.
	if len(p.Records) > 0xffff {
		return fmt.Errorf("%v records exceeds the maximum of %v", len(p.Records), 0xffff)
	}

	// Header is 0x48 bytes
	totalSize := uint32(0x48)
	// Add 6 bytes for the record list header
	totalSize += 6
	// Two more if there aren't any records
	if len(p.Records) == 0 {
		totalSize += 2
	}
	// Add 8 bytes per record.
	totalSize += uint32(len(p.Records) * 8)

	// Start the running total. We always write out the sort area, then the appinfo area.
	if len(p.SortInfo) > 0 {
		p.sortInfoOffset = totalSize
		// Add in the sortinfo area
		totalSize += uint32(len(p.SortInfo))
	} else {
		p.sortInfoOffset = 0
	}

	if len(p.AppInfo) > 0 {
		p.appInfoOffset = totalSize
		// Add in the appinfo
		totalSize += uint32(len(p.AppInfo))
	} else {
		p.appInfoOffset = 0
	}

	// Add in the sizes of all the records
	for i, r := range p.Records {
		r.offset = totalSize
		totalSize += uint32(len(r.Data))
		if len(r.Data) == 0 {
			return fmt.Errorf("Record %v has a size of 0!", i)
		}
	}

	// Too big? We're gonna call 2G our limit, since that way we can
	// dodge any nasty sign bit issues.
	if totalSize > 0x7fffffff {
		return fmt.Errorf("Total filesize of %v exceeds max of 2G", totalSize)
	}

	return nil
}

func (p *Pdb) writeHeader(fh io.Writer) error {
	// Ready for the output.
	h := header{}

	copy(h.Name[:], p.Name)
	h.Attr = p.Attributes
	h.Version = p.Version

	// We always write out unix-relative times
	h.Create = uint32(p.CreateTime.Unix())
	h.Modified = uint32(p.ModTime.Unix())
	h.Backup = uint32(p.BackupTime.Unix())

	copy(h.Filetype[:], p.Filetype[0:4])
	copy(h.Creator[:], p.Creator[0:4])
	h.ModNum = p.ModNum
	h.AppInfoOffset = p.appInfoOffset
	h.SortInfoOffset = p.sortInfoOffset
	h.UniqueIdSeed = p.UniqueIdSeed

	return binary.Write(fh, binary.BigEndian, &h)
}

func (p *Pdb) writeRecordMetadata(fh io.Writer) error {
	if _, err := fh.Write([]byte{0, 0, 0, 0}); err != nil {
		return fmt.Errorf("Error writing nil nextRecordListID offset: %v", err)
	}
	var nr uint16
	nr = uint16(len(p.Records))
	if err := binary.Write(fh, binary.BigEndian, nr); err != nil {
		return fmt.Errorf("Error writing record count: %v", err)
	}
	if nr == 0 {
		if _, err := fh.Write([]byte{0, 0}); err != nil {
			return fmt.Errorf("Error writing record list pad: %v", err)
		}
	}

	for i, r := range p.Records {
		if err := binary.Write(fh, binary.BigEndian, uint32(r.offset)); err != nil {
			return fmt.Errorf("Error writing offset for record %v: %v", i, err)
		}
		var attr uint32
		attr = uint32(r.Attribs)<<24 + (r.UniqueID & 0xffffff)
		if err := binary.Write(fh, binary.BigEndian, attr); err != nil {
			return fmt.Errorf("Error writing attributes for record %v: %v", i, err)
		}
	}

	return nil
}

// writeMetadata writes out the sortinfo and appinfo data, if they're set
func (p *Pdb) writeMetadata(fh io.Writer) error {
	if len(p.AppInfo) > 0 {
		if _, err := fh.Write(p.AppInfo); err != nil {
			return fmt.Errorf("Error writing appinfo data: %v", err)
		}
	}
	if len(p.SortInfo) > 0 {
		if _, err := fh.Write(p.SortInfo); err != nil {
			return fmt.Errorf("Error writing sortinfo data: %v", err)
		}
	}
	return nil
}

// writeRecords writes out the record data.
func (p *Pdb) writeRecords(fh io.Writer) error {
	for i, r := range p.Records {
		if _, err := fh.Write(r.Data); err != nil {
			return fmt.Errorf("Error writing record %v: %v", i, err)
		}
	}
	return nil
}

func (p *Pdb) readHeader(fh io.ReadSeeker) error {
	h := header{}
	err := binary.Read(fh, binary.BigEndian, &h)
	if err != nil {
		return err
	}

	n := string(h.Name[:])
	if i := strings.Index(n, "\x00"); i != -1 {
		n = n[0:i]
	}
	p.Name = n

	p.Attributes = h.Attr
	p.Version = h.Version

	// We don't need to be clever here, unset times will be 0
	p.CreateTime = readTime(h.Create)
	p.ModTime = readTime(h.Modified)
	p.BackupTime = readTime(h.Backup)

	p.ModNum = h.ModNum
	p.appInfoOffset = h.AppInfoOffset
	p.sortInfoOffset = h.SortInfoOffset
	p.Creator = string(h.Creator[:])
	p.Filetype = string(h.Filetype[:])
	p.UniqueIdSeed = h.UniqueIdSeed

	return nil
}

// Base date for MOBI. Jan 1, 1904.
const oldDateSecs = -2082844800

// readTime takes the first four bytes of the passed in byte slice and
// interprets them as a mobi datestamp.
func readTime(rawTime uint32) time.Time {
	// Did we get a 0? If so just return an empty time struct.
	if rawTime == 0 {
		return time.Time{}
	}
	// A time where the high bit isn't set means a time relative to unix
	// base date.
	if rawTime&0x80000000 == 0 {
		return time.Unix(int64(rawTime), 0)
	}

	// A time with the high bit is set means this is an unsigned int
	// representing a time relative to 1904. We re-parse as unsigned.
	return time.Unix(int64(rawTime)+oldDateSecs, 0)
}
