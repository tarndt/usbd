package ramdisk

import (
	"io"
	"sync/atomic"

	"github.com/tarndt/usbd/pkg/usbdlib"
	"github.com/tarndt/usbd/pkg/util/consterr"
)

const errClosed = consterr.ConstErr("Device is shutdown")

//RAMDisk is a simple memory (heap) backed user-space block device
type RAMDisk struct {
	disk []byte
	size int
	usbdlib.DefaultBlockSize

	atomicOnline uint64
}

//NewRAMDisk constructs a memory backed user-space device of the provided size
func NewRAMDisk(size int64) *RAMDisk {
	return &RAMDisk{
		disk:         make([]byte, int(size)),
		size:         int(size),
		atomicOnline: 1,
	}
}

//Size of this device in bytes
func (rdsk *RAMDisk) Size() int64 {
	return int64(rdsk.size)
}

//ReadAt fufills io.ReaderAt and in turn part of usbdlib.Device
func (rdsk *RAMDisk) ReadAt(buf []byte, pos int64) (count int, err error) {
	if atomic.LoadUint64(&rdsk.atomicOnline) != 1 {
		return 0, errClosed
	}

	count = len(buf)
	end := int(pos) + count
	if end > rdsk.size {
		return 0, io.EOF
	}
	copy(buf, rdsk.disk[pos:end])
	return
}

//WriteAt fufills io.WriterAt and in turn part of usbdlib.Device
func (rdsk *RAMDisk) WriteAt(buf []byte, pos int64) (count int, err error) {
	if atomic.LoadUint64(&rdsk.atomicOnline) != 1 {
		return 0, errClosed
	}

	count = len(buf)
	end := int(pos) + count
	if end > rdsk.size {
		return 0, io.ErrUnexpectedEOF
	}
	copy(rdsk.disk[pos:end], buf)
	return
}

//Trim fufills part of usbdlib.Device
func (rdsk *RAMDisk) Trim(pos int64, count int) error {
	if atomic.LoadUint64(&rdsk.atomicOnline) != 1 {
		return errClosed
	}

	return nil
}

//Flush fufills part of usbdlib.Device
func (rdsk *RAMDisk) Flush() error {
	if atomic.LoadUint64(&rdsk.atomicOnline) != 1 {
		return errClosed
	}

	return nil
}

//Close fufills io.Closer and in turn part of usbdlib.Device
func (rdsk *RAMDisk) Close() error {
	atomic.StoreUint64(&rdsk.atomicOnline, 0)
	return nil
}
