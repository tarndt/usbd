package dedupdisk

import "io"

//FlushClose is the Close() and Flush() in one interface
type FlushClose interface {
	io.Closer
	Flush() error
}

//LUNMap describes the capability to map logic block addresses to deduplicated
// blocks of data. Any series of blocks can then be seen as a series of dedup IDs
type LUNMap interface {
	GetID(block uint64) (dedupID uint64, err error)
	GetIDs(startBlock uint64, dedupIDs []uint64) error
	PutID(block uint64, dedupID uint64) error
	Size() int64
	FlushClose
}

//IDStore describes the capability to map content (the hashes of blocks) to dedup IDs
type IDStore interface {
	GetID(block []byte) (dedupID uint64, hash []byte, err error)
	PutID(hash []byte, dedupID uint64) error
	GetErrNotPresent() error
	FlushClose
}

//BlockStore describes the capability to store and retrieve the contents of blocks
// by their dedup IDs
type BlockStore interface {
	GetBlock(dedupID uint64, buf []byte) error
	PutBlock(buf []byte) (dedupID uint64, err error)
	FlushClose
}
