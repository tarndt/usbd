package filedisk

import (
	"bufio"
	"os"

	"github.com/tarndt/usbd/errs"
	"github.com/tarndt/usbd/usbdlib"
)

type FileDisk struct {
	*os.File
	size int64
	usbdlib.DefaultBlockSize
}

func New(filename string, ifCreateSize int64) (*FileDisk, error) {
	this := new(FileDisk)
	var err error
	if this.File, err = os.OpenFile(filename, os.O_RDWR|os.O_CREATE, 0); err != nil {
		return nil, errs.Append(err, "Could not open backing file: %s", filename)
	}
	if info, err := this.Stat(); err != nil {
		return nil, errs.Append(err, "Could not stat backing file: %s", filename)
	} else if size := info.Size(); size < 1 { //Create disk file
		strm := bufio.NewWriter(this)
		for i := int64(0); i < ifCreateSize; i++ {
			if err = strm.WriteByte(0); err != nil {
				return nil, errs.Append(err, "Could not zero fill backing file: %s, write failed", filename)
			}
		}
		if err = strm.Flush(); err != nil {
			return nil, errs.Append(err, "Could not zero fill backing file: %s, flush failed", filename)
		}
		if err = this.Sync(); err != nil {
			return nil, errs.Append(err, "Could not zero fill backing file: %s, sync failed", filename)
		}
		this.size = ifCreateSize
	} else { //disk file exists
		this.size = size
	}
	return this, nil
}

func (this *FileDisk) Size() int64 {
	return this.size
}

func (this *FileDisk) Trim(pos int64, count int) error {
	return nil
}

func (this *FileDisk) Flush() error {
	return this.Sync()
}
