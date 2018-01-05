package impls

import (
	"log"
	"os"
	"sync"
	"time"

	"github.com/tarndt/usbd/errs"
)

const syncDelay = time.Second * 5

type FileBlockStore struct {
	file *os.File

	pendingWrites map[uint64]*sync.Cond
	nextId        uint64
	nextPos       int64
	writeState    sync.RWMutex

	syncCh chan bool

	blockSize uint64
	zeroBlock []byte
}

func NewFileBlockStore(filename string, blockSize int64) (*FileBlockStore, error) {
	this := &FileBlockStore{
		blockSize:     uint64(blockSize),
		syncCh:        make(chan bool, 1),
		zeroBlock:     make([]byte, blockSize),
		pendingWrites: make(map[uint64]*sync.Cond, 757),
	}
	var err error
	if this.file, err = os.OpenFile(filename, os.O_RDWR|os.O_CREATE, 0); err != nil {
		return nil, errs.Append(err, "Could not open backing file: %s", filename)
	}
	info, err := this.file.Stat()
	if err != nil {
		return nil, errs.Append(err, "Could not stat backing file: %s", filename)
	}
	this.nextId = uint64(info.Size() / blockSize)
	go this.fsyncWorker()
	return this, nil
}

func (this *FileBlockStore) GetBlock(dedupId uint64, buf []byte) (err error) {
	//Check if this is the zero block
	if dedupId == zeroBlockId {
		copy(buf, this.zeroBlock)
		return nil
	}
	//Ensure we are safe to read
	var written *sync.Cond
	this.writeState.RLock()
	for written = this.pendingWrites[dedupId]; written != nil; written = this.pendingWrites[dedupId] {
		written.Wait()
	}
	this.writeState.RUnlock()
	//Read, we don't need the lock beacuse once written, blocks are immutable
	_, err = this.file.ReadAt(buf, int64(dedupId*this.blockSize))
	return
}

func (this *FileBlockStore) PutBlock(buf []byte) (dedupId uint64, err error) {
	//Update write state
	var pos int64
	var writeWaiters = sync.NewCond(&this.writeState)
	this.writeState.Lock()
	pos, dedupId = this.nextPos, this.nextId
	this.nextPos += int64(this.blockSize)
	this.nextId++
	this.pendingWrites[dedupId] = writeWaiters
	this.writeState.Unlock()
	//Perform write
	_, err = this.file.WriteAt(buf, pos)
	this.writeState.Lock()
	delete(this.pendingWrites, dedupId)
	this.writeState.Unlock()
	writeWaiters.Broadcast()
	if err != nil {
		return 0, err
	}
	select {
	case this.syncCh <- true:
	default:
	}
	return dedupId, nil
}

func (this *FileBlockStore) PutBlockAt(buf []byte, blockId uint64) (dedupId uint64, err error) {
	//Update write state
	var writeWaiters = sync.NewCond(&this.writeState)
	this.writeState.Lock()
	this.pendingWrites[dedupId] = writeWaiters
	this.writeState.Unlock()
	//Perform write
	_, err = this.file.WriteAt(buf, int64(blockId*this.blockSize))
	this.writeState.Lock()
	delete(this.pendingWrites, dedupId)
	this.writeState.Unlock()
	writeWaiters.Broadcast()
	if err != nil {
		return 0, err
	}
	select {
	case this.syncCh <- true:
	default:
	}
	return dedupId, nil
}

func (this *FileBlockStore) Flush() error {
	return this.file.Sync()
}

func (this *FileBlockStore) Close() error {
	return this.file.Close()
}

func (this *FileBlockStore) fsyncWorker() {
	var err error
	var looping bool
	for {
		<-this.syncCh
		time.Sleep(syncDelay)
		if err = this.Flush(); err != nil {
			log.Printf("FileBlockStore::fsyncWorker(): WARNING: Error syncing file; Details: %s", err)
		}
		looping = true
		for looping {
			select {
			case <-this.syncCh:
			default:
				looping = false
			}
		}
	}
}
