package impls

import (
	"bufio"
	"encoding/binary"
	"io"
	"os"
	"reflect"
	"unsafe"

	"usbd/errs"

	"launchpad.net/gommap"
)

type MmapLUNmap struct {
	file     *os.File
	rawBytes gommap.MMap
	ids      []uint64
	size     int64
}

func NewMmapLUNmap(filename string, lunBlockSize int64, ifCreateSize int64) (*MmapLUNmap, error) {
	idCount := ifCreateSize / lunBlockSize
	//Create or open backing file
	file, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE, 0)
	if err != nil {
		return nil, errs.Append(err, "Could not open backing file: %s", filename)
	}
	if info, err := file.Stat(); err != nil {
		return nil, errs.Append(err, "Could not stat backing file: %s", filename)
	} else if size := info.Size(); size < 1 { //Create disk file
		strm := bufio.NewWriter(file)
		zeroIdBytes := make([]byte, 8)
		binary.LittleEndian.PutUint64(zeroIdBytes, zeroBlockId)
		for i := int64(0); i < idCount; i++ {
			if _, err = strm.Write(zeroIdBytes); err != nil {
				return nil, errs.Append(err, "Could not zero fill backing file: %s, write failed", filename)
			}
		}
		if err = strm.Flush(); err != nil {
			return nil, errs.Append(err, "Could not zero fill backing file: %s, flush failed", filename)
		}
		if err = file.Sync(); err != nil {
			return nil, errs.Append(err, "Could not zero fill backing file: %s, sync failed", filename)
		}
	}
	mmap, err := gommap.Map(file.Fd(), gommap.PROT_READ|gommap.PROT_WRITE, gommap.MAP_SHARED)
	if err != nil {
		return nil, errs.Append(err, "Could not mmap backing file")
	}
	bytesHdr := *((*reflect.SliceHeader)(unsafe.Pointer(&mmap)))
	longsHdr := reflect.SliceHeader{Data: bytesHdr.Data, Len: bytesHdr.Len / 8, Cap: bytesHdr.Cap / 8}
	return &MmapLUNmap{file, mmap, *(*[]uint64)(unsafe.Pointer(&longsHdr)), idCount * lunBlockSize}, nil
}

func (this *MmapLUNmap) GetId(block uint64) (dedupId uint64, err error) {
	if block > uint64(len(this.ids)) {
		return 0, errs.Append(io.EOF, "Out of bounds read")
	}
	return this.ids[block], nil
}

func (this *MmapLUNmap) GetIds(startBlock uint64, dedupIds []uint64) error {
	end := startBlock + uint64(len(dedupIds))
	if end > uint64(len(this.ids)) {
		return errs.Append(io.EOF, "Out of bounds read")
	}
	copy(dedupIds, this.ids[startBlock:end])
	return nil
}

func (this *MmapLUNmap) PutId(block uint64, dedupId uint64) error {
	if block > uint64(this.size) {
		return errs.Append(io.ErrUnexpectedEOF, "Out of bounds read")
	}
	this.ids[block] = dedupId
	this.rawBytes.Sync(gommap.MS_ASYNC)
	return nil
}

func (this *MmapLUNmap) Size() int64 {
	return this.size
}

func (this *MmapLUNmap) Close() error {
	flushErr := this.Flush()
	err := this.file.Close()
	if err == nil && flushErr != nil {
		err = flushErr
	}
	if err != nil {
		return errs.Append(err, "Could not close backing file")
	}
	return nil
}

func (this *MmapLUNmap) Flush() error {
	return this.rawBytes.Sync(gommap.MS_SYNC)
}
