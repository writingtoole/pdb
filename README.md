# pdb
A library to read and write Palm DB files

These files are documented in a PDF at http://www.palmos.com/dev/tech/docs/fileformats.zip. See the Internet Archive for a copy.

This package fully implements reading and writing PDB files. It doesn't parse the contents or flags; that's the responsibility of your code.

This package also contains a fully functional implementation of the lz77 compression algorithm commonly used to compress PDB records. (Text data in .mobi files, for the most part these days) It's been tested against the C implementation in Calibre (http://calibre-ebook.com) though those tests aren't included in this package for licensing reasons.