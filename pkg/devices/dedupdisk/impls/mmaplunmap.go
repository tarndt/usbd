package impls

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"reflect"
	"unsafe"

	"github.com/tarndt/usbd/pkg/devices/dedupdisk"

	"launchpad.net/gommap"
)

type mmapLUNmap struct {
	file     *os.File
	rawBytes gommap.MMap
	ids      []uint64
	size     int64
}

//NewMmapLUNmap contructs a dedupdisk.LUNMap implemented using a mem-mapped file
func NewMmapLUNmap(filename string, lunBlockSize int64, ifCreateSize int64) (dedupdisk.LUNMap, error) {
	idCount := ifCreateSize / lunBlockSize
	//Create or open backing file
	file, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE, 0666)
	if err != nil {
		return nil, fmt.Errorf("Could not open backing file %q: %w", filename, err)
	}
	if info, err := file.Stat(); err != nil {
		return nil, fmt.Errorf("Could not stat backing file %q: %w", filename, err)
	} else if size := info.Size(); size < 1 { //Create disk file
		strm := bufio.NewWriter(file)
		zeroIDBytes := make([]byte, 8)
		binary.LittleEndian.PutUint64(zeroIDBytes, zeroBlockID)
		for i := int64(0); i < idCount; i++ {
			if _, err = strm.Write(zeroIDBytes); err != nil {
				return nil, fmt.Errorf("Could not zero fill backing file %q, write failed: %w", filename, err)
			}
		}
		if err = strm.Flush(); err != nil {
			return nil, fmt.Errorf("Could not zero fill backing file %q, flush failed: %w", filename, err)
		}
		if err = file.Sync(); err != nil {
			return nil, fmt.Errorf("Could not zero fill backing file %q, sync failed: %w", filename, err)
		}
	}
	mmap, err := gommap.Map(file.Fd(), gommap.PROT_READ|gommap.PROT_WRITE, gommap.MAP_SHARED)
	if err != nil {
		return nil, fmt.Errorf("Could not mmap backing file %q (fd %d): %w", filename, file.Fd(), err)
	}
	bytesHdr := *((*reflect.SliceHeader)(unsafe.Pointer(&mmap)))
	longsHdr := reflect.SliceHeader{Data: bytesHdr.Data, Len: bytesHdr.Len / 8, Cap: bytesHdr.Cap / 8}
	return &mmapLUNmap{file, mmap, *(*[]uint64)(unsafe.Pointer(&longsHdr)), idCount * lunBlockSize}, nil
}

func (lm *mmapLUNmap) GetID(block uint64) (dedupID uint64, err error) {
	if block > uint64(len(lm.ids)) {
		return 0, fmt.Errorf("Out of bounds read: %w", io.EOF)
	}
	return lm.ids[block], nil
}

func (lm *mmapLUNmap) GetIDs(startBlock uint64, dedupIds []uint64) error {
	end := startBlock + uint64(len(dedupIds))
	if end > uint64(len(lm.ids)) {
		return fmt.Errorf("Out of bounds read: %w", io.EOF)
	}
	copy(dedupIds, lm.ids[startBlock:end])
	return nil
}

func (lm *mmapLUNmap) PutID(block uint64, dedupID uint64) error {
	if block > uint64(lm.size) {
		return fmt.Errorf("Out of bounds write: %w", io.ErrUnexpectedEOF)
	}
	lm.ids[block] = dedupID
	lm.rawBytes.Sync(gommap.MS_ASYNC)
	return nil
}

func (lm *mmapLUNmap) Size() int64 {
	return lm.size
}

func (lm *mmapLUNmap) Close() error {
	flushErr := lm.Flush()
	err := lm.file.Close()
	if err == nil && flushErr != nil {
		err = flushErr
	}
	if err != nil {
		return fmt.Errorf("Could not close backing file: %w", err)
	}
	return nil
}

func (lm *mmapLUNmap) Flush() error {
	return lm.rawBytes.Sync(gommap.MS_SYNC)
}
