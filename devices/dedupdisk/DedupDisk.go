package dedupdisk

import (
	"io"
	"sync"

	"usbd/errs"
	"usbd/usbdlib"
)

type DedupDisk struct {
	lunMap          LUNMap
	idStore         IdStore
	blockStore      BlockStore
	blockSize, size int64
	errNotPresent   error
	usbdlib.DefaultBlockSize
}

func New(lunMap LUNMap, idStore IdStore, blockStore BlockStore) *DedupDisk {
	this := &DedupDisk{
		lunMap:        lunMap,
		idStore:       idStore,
		blockStore:    blockStore,
		errNotPresent: idStore.GetErrNotPresent(),
	}
	this.blockSize, this.size = this.BlockSize(), this.lunMap.Size()
	return this
}

func (this *DedupDisk) Size() int64 {
	return this.size
}

func (this *DedupDisk) ReadAt(buf []byte, pos int64) (count int, err error) {
	readSize := int64(len(buf))
	if pos < 0 || pos+readSize > this.size {
		return 0, errs.Append(io.EOF, "Out of bounds read")
	}
	block := uint64(pos / this.blockSize)
	if readSize > this.blockSize {
		//more common multiblock read path
		blockc := readSize / this.blockSize
		dedupIds, errors := make([]uint64, blockc), make([]error, blockc)
		if err = this.lunMap.GetIds(block, dedupIds); err != nil {
			return 0, errs.Append(err, "lunMap lookup of %d LUN blocks starting %d failed", blockc, block)
		}
		//Run our many requests
		var dedupId uint64
		var bufStart int64
		var i int
		var wg sync.WaitGroup
		wg.Add(int(blockc))
		for i, dedupId = range dedupIds {
			go func(id uint64, buffer []byte, err *error, wg *sync.WaitGroup) {
				if readError := this.blockStore.GetBlock(id, buffer); readError != nil {
					*err = errs.Append(readError, "Fetch of dedup block %d failed", dedupId)
				}
				wg.Done()
			}(dedupId, buf[bufStart:bufStart+this.blockSize], &errors[i], &wg)
			bufStart += this.blockSize
		}
		if err = getFirstError(errors); err != nil {
			return 0, err
		}
	} else {
		//fast-path for read single block
		var dedupId uint64
		if dedupId, err = this.lunMap.GetId(block); err != nil {
			return 0, errs.Append(err, "lunMap lookup of LUN block %d failed", block)
		}
		if err = this.blockStore.GetBlock(dedupId, buf); err != nil {
			return 0, errs.Append(err, "Fetch of dedup block %d from block store failed", dedupId)
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

func (this *DedupDisk) WriteAt(buf []byte, pos int64) (count int, err error) {
	writeSize := int64(len(buf))
	if pos < 0 || pos+writeSize > this.size {
		return 0, errs.Append(io.ErrUnexpectedEOF, "Out of bounds write")
	}
	if writeSize > this.blockSize {
		//more common multiblock read path
		blockc := int(writeSize / this.blockSize)
		errors := make([]error, blockc)
		var wg sync.WaitGroup
		wg.Add(blockc)
		for i, bufPos := 0, int64(0); i < blockc; i++ {
			/*go*/ func(buffer []byte, pos int64, err *error) { //TODO limit max goroutines
				if _, writeErr := this.WriteAt(buffer, pos); err != nil {
					*err = writeErr
				}
				wg.Done()
			}(buf[bufPos:bufPos+this.blockSize], pos, &errors[i])
			pos += this.blockSize
			bufPos += this.blockSize
		}
		wg.Wait()
		if err = getFirstError(errors); err != nil {
			return 0, err
		}
	} else {
		//fast-path for write single block
		var dedupId uint64
		var hash []byte
		block := uint64(pos / this.blockSize)
		if dedupId, hash, err = this.idStore.GetId(buf); err == this.errNotPresent {
			if dedupId, err = this.blockStore.PutBlock(buf); err != nil {
				return 0, errs.Append(err, "Addition of dedup block %d to block store failed", dedupId)
			}
			if this.idStore.PutId(hash, dedupId); err != nil {
				return 0, errs.Append(err, "Addition of dedup block %d to idStore store failed", dedupId)
			}
		} else if err != nil {
			return 0, errs.Append(err, "ID store lookup of write block %d failed", block)
		}
		if err = this.lunMap.PutId(block, dedupId); err != nil {
			return 0, errs.Append(err, "Addition of dedup block ID %d to lunMap failed", dedupId)
		}
	}
	return int(writeSize), nil
}

func (this *DedupDisk) Trim(pos int64, count int) error {
	return nil //TODO
}

func (this *DedupDisk) Flush() error {
	errCh := make(chan error, 6)
	var wg sync.WaitGroup
	wg.Add(3)
	flush := func(resouce FlushClose) {
		defer wg.Done()
		if err := resouce.Flush(); err != nil {
			errCh <- err
		}
	}
	go flush(this.lunMap)
	go flush(this.idStore)
	go flush(this.blockStore)
	wg.Wait()
	select { //return first error
	case err := <-errCh:
		return errs.Append(err, "One or more errors occured")
	default:
	}
	return nil
}

func (this *DedupDisk) Close() error {
	errCh := make(chan error, 6)
	var wg sync.WaitGroup
	wg.Add(3)
	cleanup := func(resouce FlushClose) {
		defer wg.Done()
		if err := resouce.Flush(); err != nil {
			errCh <- err
		}
		if err := resouce.Close(); err != nil {
			errCh <- err
		}
	}
	go cleanup(this.lunMap)
	go cleanup(this.idStore)
	go cleanup(this.blockStore)
	wg.Wait()
	select { //return first error
	case err := <-errCh:
		return errs.Append(err, "One or more errors occured")
	default:
	}
	return nil
}
