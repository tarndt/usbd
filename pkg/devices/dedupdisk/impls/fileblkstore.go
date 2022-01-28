package impls

import (
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/tarndt/usbd/pkg/devices/dedupdisk"
)

const syncDelay = time.Second * 5

type fileBlockStore struct {
	file *os.File

	pendingWrites map[uint64]*sync.Cond
	nextID        uint64
	nextPos       int64
	writeState    sync.RWMutex

	syncCh chan bool

	blockSize uint64
	zeroBlock []byte
}

//NewFileBlockStore constructs a dedupdisk.BlockStore backed by a simple file
func NewFileBlockStore(filename string, blockSize int64) (dedupdisk.BlockStore, error) {
	fbs := &fileBlockStore{
		blockSize:     uint64(blockSize),
		syncCh:        make(chan bool, 1),
		zeroBlock:     make([]byte, blockSize),
		pendingWrites: make(map[uint64]*sync.Cond, 757),
	}
	var err error
	if fbs.file, err = os.OpenFile(filename, os.O_RDWR|os.O_CREATE, 0666); err != nil {
		return nil, fmt.Errorf("Could not open backing file %q: %w", filename, err)
	}
	info, err := fbs.file.Stat()
	if err != nil {
		return nil, fmt.Errorf("Could not stat backing file %q: %w", filename, err)
	}
	fbs.nextID = uint64(info.Size() / blockSize)
	go fbs.fsyncWorker()
	return fbs, nil
}

func (fbs *fileBlockStore) GetBlock(dedupID uint64, buf []byte) (err error) {
	//Check if fbs is the zero block
	if dedupID == zeroBlockID {
		copy(buf, fbs.zeroBlock)
		return nil
	}
	//Ensure we are safe to read
	var written *sync.Cond
	fbs.writeState.RLock()
	for written = fbs.pendingWrites[dedupID]; written != nil; written = fbs.pendingWrites[dedupID] {
		written.Wait()
	}
	fbs.writeState.RUnlock()
	//Read, we don't need the lock because once written, blocks are immutable
	_, err = fbs.file.ReadAt(buf, int64(dedupID*fbs.blockSize))
	return
}

func (fbs *fileBlockStore) PutBlock(buf []byte) (dedupID uint64, err error) {
	//Update write state
	var pos int64
	var writeWaiters = sync.NewCond(&fbs.writeState)
	fbs.writeState.Lock()
	pos, dedupID = fbs.nextPos, fbs.nextID
	fbs.nextPos += int64(fbs.blockSize)
	fbs.nextID++
	fbs.pendingWrites[dedupID] = writeWaiters
	fbs.writeState.Unlock()
	//Perform write
	_, err = fbs.file.WriteAt(buf, pos)
	fbs.writeState.Lock()
	delete(fbs.pendingWrites, dedupID)
	fbs.writeState.Unlock()
	writeWaiters.Broadcast()
	if err != nil {
		return 0, err
	}
	select {
	case fbs.syncCh <- true:
	default:
	}
	return dedupID, nil
}

func (fbs *fileBlockStore) PutBlockAt(buf []byte, blockID uint64) (dedupID uint64, err error) {
	//Update write state
	var writeWaiters = sync.NewCond(&fbs.writeState)
	fbs.writeState.Lock()
	fbs.pendingWrites[dedupID] = writeWaiters
	fbs.writeState.Unlock()
	//Perform write
	_, err = fbs.file.WriteAt(buf, int64(blockID*fbs.blockSize))
	fbs.writeState.Lock()
	delete(fbs.pendingWrites, dedupID)
	fbs.writeState.Unlock()
	writeWaiters.Broadcast()
	if err != nil {
		return 0, err
	}
	select {
	case fbs.syncCh <- true:
	default:
	}
	return dedupID, nil
}

func (fbs *fileBlockStore) Flush() error {
	return fbs.file.Sync()
}

func (fbs *fileBlockStore) Close() error {
	return fbs.file.Close()
}

func (fbs *fileBlockStore) fsyncWorker() {
	var err error
	var looping bool
	for {
		<-fbs.syncCh
		time.Sleep(syncDelay)
		if err = fbs.Flush(); err != nil {
			log.Printf("FileBlockStore::fsyncWorker(): WARNING: Error syncing file; Details: %s", err)
		}
		looping = true
		for looping {
			select {
			case <-fbs.syncCh:
			default:
				looping = false
			}
		}
	}
}
