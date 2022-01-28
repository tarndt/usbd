package filedisk

import (
	"bufio"
	"fmt"
	"os"

	"github.com/tarndt/usbd/pkg/usbdlib"
)

//FileDisk is a simple file backed user-space block device
type FileDisk struct {
	*os.File
	SizeBytes int64
	usbdlib.DefaultBlockSize
}

//NewFileDisk is the constructor for simple file backed devices
func NewFileDisk(filename string, ifCreateSize int64) (*FileDisk, error) {
	fdsk := new(FileDisk)
	var err error
	if fdsk.File, err = os.OpenFile(filename, os.O_RDWR|os.O_CREATE, 0); err != nil {
		return nil, fmt.Errorf("Could not open backing file %q: %w", filename, err)
	}
	if info, err := fdsk.Stat(); err != nil {
		return nil, fmt.Errorf("Could not stat backing file %q: %w", filename, err)
	} else if size := info.Size(); size < 1 { //Create disk file
		strm := bufio.NewWriter(fdsk)
		for i := int64(0); i < ifCreateSize; i++ {
			if err = strm.WriteByte(0); err != nil {
				return nil, fmt.Errorf("Could not zero fill backing file %q, write failed: %w", filename, err)
			}
		}
		if err = strm.Flush(); err != nil {
			return nil, fmt.Errorf("Could not zero fill backing file: %q, flush failed: %w", filename, err)
		}
		if err = fdsk.Sync(); err != nil {
			return nil, fmt.Errorf("Could not zero fill backing file: %q, sync failed: %w", filename, err)
		}
		fdsk.SizeBytes = ifCreateSize
	} else { //disk file exists
		fdsk.SizeBytes = size
	}
	return fdsk, nil
}

//Size of this device in bytes
func (fdsk *FileDisk) Size() int64 {
	return fdsk.SizeBytes
}

//Trim fufills part of usbdlib.Device
func (*FileDisk) Trim(pos int64, count int) error {
	return nil
}

//Flush fufills part of usbdlib.Device
func (fdsk *FileDisk) Flush() error {
	return fdsk.Sync()
}
