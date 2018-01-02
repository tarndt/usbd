package dedupdisk

import "io"

type FlushClose interface {
	io.Closer
	Flush() error
}

type LUNMap interface {
	GetId(block uint64) (dedupId uint64, err error)
	GetIds(startBlock uint64, dedupIds []uint64) error
	PutId(block uint64, dedupId uint64) error
	Size() int64
	FlushClose
}

type IdStore interface {
	GetId(block []byte) (dedupId uint64, hash []byte, err error)
	PutId(hash []byte, dedupId uint64) error
	GetErrNotPresent() error
	FlushClose
}

type BlockStore interface {
	GetBlock(dedupId uint64, buf []byte) error
	PutBlock(buf []byte) (dedupId uint64, err error)
	FlushClose
}
