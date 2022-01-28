package dedupdisk

//NOTE: This device is of older and is generally lower code quality than the
// simple devices (RAMDisk, FileDisk) which have been updated or the new ObjectStore
// device. If looking for reference examples refer to those implementations instead.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/tarndt/usbd/pkg/usbdlib"
)

var errShutdown = errors.New("Device is shutdown")

type dedupDisk struct {
	lunMap          LUNMap
	idStore         IDStore
	blockStore      BlockStore
	blockSize, size int64
	errNotPresent   error
	usbdlib.DefaultBlockSize

	optMu     sync.RWMutex
	ctx       context.Context
	ctxCancel context.CancelFunc
	closeOnce sync.Once
}

//NewDedupDisk is the constructor for file backed devices that use PebbleDB to dedupliacate
// their blocks
func NewDedupDisk(lunMap LUNMap, idStore IDStore, blockStore BlockStore) usbdlib.Device {
	ctx, cancel := context.WithCancel(context.Background())

	this := &dedupDisk{
		lunMap:        lunMap,
		idStore:       idStore,
		blockStore:    blockStore,
		errNotPresent: idStore.GetErrNotPresent(),
		ctx:           ctx,
		ctxCancel:     cancel,
	}
	this.blockSize, this.size = this.BlockSize(), this.lunMap.Size()
	return this
}

//Size of this device in bytes
func (dd *dedupDisk) Size() int64 {
	return dd.size
}

//ReadAt fufills io.ReaderAt and in turn part of usbdlib.Device
func (dd *dedupDisk) ReadAt(buf []byte, pos int64) (count int, err error) {
	dd.optMu.RLock()
	defer dd.optMu.RUnlock()

	if dd.ctx.Err() != nil {
		return -1, errShutdown
	}

	readSize := int64(len(buf))
	if pos < 0 {
		return 0, fmt.Errorf("Out of bounds read with negative position")
	} else if pos+readSize > dd.size {
		return 0, io.EOF
	}

	block := uint64(pos / dd.blockSize)
	if readSize > dd.blockSize {
		//more common multiblock read path
		blockc := readSize / dd.blockSize
		dedupIds, errors := make([]uint64, blockc), make([]error, blockc)
		if err = dd.lunMap.GetIDs(block, dedupIds); err != nil {
			return 0, fmt.Errorf("lunMap lookup of %d LUN blocks starting %d failed: %w", blockc, block, err)
		}
		//Run our many requests
		var dedupID uint64
		var bufStart int64
		var i int
		var wg sync.WaitGroup
		wg.Add(int(blockc))
		for i, dedupID = range dedupIds {
			go func(id uint64, buffer []byte, err *error, wg *sync.WaitGroup) {
				if readError := dd.blockStore.GetBlock(id, buffer); readError != nil {
					*err = fmt.Errorf("Fetch of dedup block %d failed: %w", dedupID, readError)
				}
				wg.Done()
			}(dedupID, buf[bufStart:bufStart+dd.blockSize], &errors[i], &wg)
			bufStart += dd.blockSize
		}

		wg.Wait()
		if err = getFirstError(errors); err != nil {
			return 0, err
		}
	} else {
		//fast-path for read single block
		var dedupID uint64
		if dedupID, err = dd.lunMap.GetID(block); err != nil {
			return 0, fmt.Errorf("lunMap lookup of LUN block %d failed: %w", block, err)
		}
		if err = dd.blockStore.GetBlock(dedupID, buf); err != nil {
			return 0, fmt.Errorf("Fetch of dedup block %d from block store failed: %w", dedupID, err)
		}
	}
	return int(readSize), nil
}

func getFirstError(errors []error) (err error) {
	for _, err = range errors {
		if err != nil {
			break
		}
	}
	return
}

//WriteAt fufills io.WriterAt and in turn part of usbdlib.Device
func (dd *dedupDisk) WriteAt(buf []byte, pos int64) (count int, err error) {
	dd.optMu.RLock()
	defer dd.optMu.RUnlock()

	if dd.ctx.Err() != nil {
		return -1, errShutdown
	}

	writeSize := int64(len(buf))
	if pos < 0 {
		return 0, fmt.Errorf("Out of bounds write with negative position")
	} else if pos+writeSize > dd.size {
		return 0, io.ErrUnexpectedEOF
	}

	if writeSize > dd.blockSize {
		//more common multiblock read path
		blockc := int(writeSize / dd.blockSize)
		errors := make([]error, blockc)
		var wg sync.WaitGroup
		wg.Add(blockc)
		for i, bufPos := 0, int64(0); i < blockc; i++ {
			/*go*/ func(buffer []byte, pos int64, err *error) { //TODO limit max goroutines
				if _, writeErr := dd.WriteAt(buffer, pos); err != nil {
					*err = writeErr
				}
				wg.Done()
			}(buf[bufPos:bufPos+dd.blockSize], pos, &errors[i])
			pos += dd.blockSize
			bufPos += dd.blockSize
		}

		wg.Wait()
		if err = getFirstError(errors); err != nil {
			return 0, err
		}
	} else {
		//fast-path for write single block
		var dedupID uint64
		var hash []byte
		block := uint64(pos / dd.blockSize)
		if dedupID, hash, err = dd.idStore.GetID(buf); err == dd.errNotPresent {
			if dedupID, err = dd.blockStore.PutBlock(buf); err != nil {
				return 0, fmt.Errorf("Addition of dedup block %d to block store failed: %w", dedupID, err)
			}
			if dd.idStore.PutID(hash, dedupID); err != nil {
				return 0, fmt.Errorf("Addition of dedup block %d to idStore store failed: %w", dedupID, err)
			}
		} else if err != nil {
			return 0, fmt.Errorf("ID store lookup of write block %d failed: %w", block, err)
		}
		if err = dd.lunMap.PutID(block, dedupID); err != nil {
			return 0, fmt.Errorf("Addition of dedup block ID %d to lunMap failed: %w", dedupID, err)
		}
	}
	return int(writeSize), nil
}

//Trim fufills part of usbdlib.Device
func (*dedupDisk) Trim(pos int64, count int) error {
	return nil //TODO
}

//Flush fufills part of usbdlib.Device
func (dd *dedupDisk) Flush() error {
	dd.optMu.RLock()
	defer dd.optMu.RUnlock()

	if dd.ctx.Err() != nil {
		return errShutdown
	}

	errCh := make(chan error, 6)
	var wg sync.WaitGroup
	wg.Add(3)
	flush := func(resouce FlushClose) {
		defer wg.Done()
		if err := resouce.Flush(); err != nil {
			errCh <- err
		}
	}
	go flush(dd.lunMap)
	go flush(dd.idStore)
	go flush(dd.blockStore)
	wg.Wait()
	select { //return first error
	case err := <-errCh:
		return fmt.Errorf("One or more errors occurred: %w", err)
	default:
	}
	return nil
}

//ReadAt fufills io.Closer and in turn part of usbdlib.Device
func (dd *dedupDisk) Close() (err error) {
	dd.closeOnce.Do(func() {
		dd.optMu.Lock()
		defer dd.optMu.Unlock()

		dd.ctxCancel()
		err = dd.close()
	})
	return err
}

func (dd *dedupDisk) close() error {
	errCh := make(chan error, 6)
	var wg sync.WaitGroup
	wg.Add(3)
	cleanup := func(resource FlushClose) {
		defer wg.Done()
		if err := resource.Flush(); err != nil {
			errCh <- err
		}
		if err := resource.Close(); err != nil {
			errCh <- err
		}
	}
	go cleanup(dd.lunMap)
	go cleanup(dd.idStore)
	go cleanup(dd.blockStore)
	wg.Wait()
	select { //return first error
	case err := <-errCh:
		return fmt.Errorf("One or more errors occurred: %w", err)
	default:
	}
	return nil
}
