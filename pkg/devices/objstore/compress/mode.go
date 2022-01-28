package compress

import (
	"compress/gzip"
	"fmt"
	"io"

	"github.com/klauspost/compress/s2"
)

//Mode represents a compression mode
type Mode uint8

//Enumerate available modes and their textual names
const (
	ModeIdentity Mode = iota
	ModeUnknown
	ModeS2
	ModeGzip

	ModeIdentityName = "identity"
	ModeS2Name       = "s2"
	ModeGzipName     = "gzip"
	ModeUknownName   = "unknown"
)

//ModeFromName constructs a Mode from a textual name
func ModeFromName(name string) Mode {
	switch name {
	case "", ModeIdentityName:
		return ModeIdentity
	case ModeS2Name:
		return ModeS2
	case ModeGzipName:
		return ModeGzip
	}
	return ModeUnknown
}

//AlgoName returns the textual name of a Mode
func (m Mode) AlgoName() string {
	switch m {
	case ModeIdentity:
		return ModeIdentityName
	case ModeS2:
		return ModeS2Name
	case ModeGzip:
		return ModeGzipName
	}
	return ModeUknownName
}

//String is a synonym for AlgoName
func (m Mode) String() string {
	return m.AlgoName()
}

//NewReader constructs a reader wrapper that applies this mode's decompression
func (m Mode) NewReader(rdr io.Reader) (io.Reader, error) {
	switch m {
	case ModeIdentity:
		return rdr, nil
	case ModeGzip:
		return gzip.NewReader(rdr)
	case ModeS2:
		return s2.NewReader(rdr), nil
	}
	return nil, fmt.Errorf("Cannot create decompressor for unknown compression mode")
}

//NewWriter constructs a writer wrapper that applies this mode's compression
func (m Mode) NewWriter(wtr io.WriteCloser) (io.WriteCloser, error) {
	switch m {
	case ModeIdentity:
		return wtr, nil
	case ModeGzip:
		return gzip.NewWriter(wtr), nil
	case ModeS2:
		return s2.NewWriter(wtr), nil
	}
	return nil, fmt.Errorf("Cannot create decompressor for unknown compression mode")
}
