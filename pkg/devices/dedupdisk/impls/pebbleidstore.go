package impls

import (
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"hash"
	"sync"

	"github.com/tarndt/usbd/pkg/devices/dedupdisk"
	"github.com/tarndt/usbd/pkg/util"

	"github.com/cockroachdb/pebble"
	"github.com/cockroachdb/pebble/bloom"
)

type pebbleIDStore struct {
	dbPath    string
	db        *pebble.DB
	hashPool  sync.Pool
	writeOpts *pebble.WriteOptions
	closeOnce sync.Once
}

//NewPebbleIDStore constructs a dedupdisk.IDStore implemented using PebbleBD
func NewPebbleIDStore(dbPath string, cacheBytes, bloomBitsPerEntry int) (dedupdisk.IDStore, error) {
	opts := &pebble.Options{Cache: pebble.NewCache(int64(cacheBytes))}
	if bloomBitsPerEntry > 0 {
		levelOpts := new(pebble.LevelOptions).EnsureDefaults()
		levelOpts.FilterPolicy = bloom.FilterPolicy(bloomBitsPerEntry)
		opts.Levels = append(opts.Levels, *levelOpts)
	}

	db, err := pebble.Open(dbPath, opts)
	if err != nil {
		return nil, fmt.Errorf("Could not open database %q: %w", dbPath, err)
	}

	return &pebbleIDStore{
		dbPath:    dbPath,
		db:        db,
		hashPool:  sync.Pool{New: newSHA1},
		writeOpts: &pebble.WriteOptions{Sync: false},
	}, nil
}

func newSHA1() interface{} {
	return sha1.New()
}

func (ps *pebbleIDStore) GetID(block []byte) (dedupID uint64, sha1Hash []byte, err error) {
	if util.IsZeros(block) {
		return zeroBlockID, nil, nil
	}
	hashGen := ps.hashPool.Get().(hash.Hash)
	hashGen.Write(block)
	sha1Hash = hashGen.Sum(nil)
	hashGen.Reset()

	rawVal, closeVal, err := ps.db.Get(sha1Hash)
	switch err {
	case pebble.ErrNotFound:
		return 0, sha1Hash, errNotPresent
	case nil:
		dedupID = binary.LittleEndian.Uint64(rawVal)
		closeVal.Close()
		ps.hashPool.Put(hashGen)

	default:
		ps.hashPool.Put(hashGen)
		return 0, nil, fmt.Errorf("Database get %q failed: %w", hex.EncodeToString(sha1Hash), err)
	}

	return dedupID, nil, nil
}

func (ps *pebbleIDStore) PutID(sha1Hash []byte, dedupID uint64) error {
	rawVal := make([]byte, 8)
	binary.LittleEndian.PutUint64(rawVal, dedupID)
	if err := ps.db.Set(sha1Hash, rawVal, ps.writeOpts); err != nil {
		return fmt.Errorf("Database put %q failed: %w", hex.EncodeToString(sha1Hash), err)
	}
	return nil
}

func (ps *pebbleIDStore) GetErrNotPresent() error {
	return errNotPresent
}

func (ps *pebbleIDStore) Flush() error {
	return ps.db.Flush()
}

func (ps *pebbleIDStore) Close() (err error) {
	ps.closeOnce.Do(func() {
		err = ps.db.Close()
	})
	return err
}
