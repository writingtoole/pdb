# pdb
A library to read and write Palm DB files

These files are documented in a PDF at http://www.palmos.com/dev/tech/docs/fileformats.zip. See the Internet Archive for a copy.

Note that at the moment the pdb read/write functionality is very rudimentary. The lz77 implementation, however, works (or is at least internally consistent, which is reasonably important for a compression algorithm).