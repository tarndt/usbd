package usbdlib

//Device represents the contact required of Go user-space device imlplementations
type Device interface {
	Size() int64
	BlockSize() int64
	ReadAt(buf []byte, pos int64) (count int, err error)
	WriteAt(buf []byte, pos int64) (count int, err error)
	Trim(pos int64, count int) error
	Flush() error
	Close() error
}
