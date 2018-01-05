package ramdisk

import (
	"io"

	"github.com/tarndt/usbd/usbdlib"
)

type RAMDisk struct {
	disk []byte
	size int
	usbdlib.DefaultBlockSize
}

func New(size int64) *RAMDisk {
	return &RAMDisk{
		disk: make([]byte, int(size)),
		size: int(size),
	}
}

func (this *RAMDisk) Size() int64 {
	return int64(this.size)
}

func (this *RAMDisk) ReadAt(buf []byte, pos int64) (count int, err error) {
	count = len(buf)
	end := int(pos) + count
	if end > this.size {
		return 0, io.EOF
	}
	copy(buf, this.disk[pos:end])
	return
}

func (this *RAMDisk) WriteAt(buf []byte, pos int64) (count int, err error) {
	count = len(buf)
	end := int(pos) + count
	if end > this.size {
		return 0, io.ErrUnexpectedEOF
	}
	copy(this.disk[pos:end], buf)
	return
}

func (this *RAMDisk) Trim(pos int64, count int) error {
	return nil
}

func (this *RAMDisk) Flush() error {
	return nil
}

func (this *RAMDisk) Close() error {
	return nil
}
