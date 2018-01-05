package impls

import (
	"crypto/sha1"
	"encoding/binary"
	"hash"
	"sync"

	"github.com/tarndt/usbd/errs"

	"github.com/alberts/gorocks"
)

type RocksIdStore struct {
	dbPath    string
	db        *gorocks.DB
	hashPool  sync.Pool
	openOpts  *gorocks.Options
	readOpts  *gorocks.ReadOptions
	writeOpts *gorocks.WriteOptions
}

func NewRocksIdStore(dbPath string, cacheBytes, bloomBitsPerEntry int) (*RocksIdStore, error) {
	opts := gorocks.NewOptions()
	opts.SetCache(gorocks.NewLRUCache(cacheBytes))
	opts.SetCreateIfMissing(true)
	if bloomBitsPerEntry > 0 {
		opts.SetFilterPolicy(gorocks.NewBloomFilter(bloomBitsPerEntry))
	}
	db, err := gorocks.Open(dbPath, opts)
	if err != nil {
		return nil, errs.Append(err, "Could not open database")
	}
	return &RocksIdStore{
		dbPath:    dbPath,
		openOpts:  opts,
		db:        db,
		hashPool:  sync.Pool{New: NewSHA1},
		readOpts:  gorocks.NewReadOptions(),
		writeOpts: gorocks.NewWriteOptions(),
	}, nil
}

func NewSHA1() interface{} {
	return sha1.New()
}

func (this *RocksIdStore) GetId(block []byte) (dedupId uint64, sha1Hash []byte, err error) {
	if isZeros(block) {
		return zeroBlockId, nil, nil
	}
	hashGen := this.hashPool.Get().(hash.Hash)
	hashGen.Write(block)
	sha1Hash = hashGen.Sum(nil)
	hashGen.Reset()
	rawVal, err := this.db.Get(this.readOpts, sha1Hash)
	if err != nil {
		this.hashPool.Put(hashGen)
		return 0, nil, errs.Append(err, "Database get failed")
	} else if rawVal == nil {
		return 0, sha1Hash, errNotPresent
	}
	this.hashPool.Put(hashGen)
	return binary.LittleEndian.Uint64(rawVal), nil, nil
}

func (this *RocksIdStore) PutId(sha1Hash []byte, dedupId uint64) error {
	rawVal := make([]byte, 8)
	binary.LittleEndian.PutUint64(rawVal, dedupId)
	if err := this.db.Put(this.writeOpts, sha1Hash, rawVal); err != nil {
		return errs.Append(err, "Database put failed")
	}
	return nil
}

func (this *RocksIdStore) GetErrNotPresent() error {
	return errNotPresent
}

func (this *RocksIdStore) Flush() error {
	this.db.Close()
	var err error
	this.db, err = gorocks.Open(this.dbPath, this.openOpts)
	return err
}

func (this *RocksIdStore) Close() error {
	this.readOpts.Close()
	this.writeOpts.Close()
	this.db.Close()
	return nil
}
