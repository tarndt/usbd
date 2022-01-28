package strms

import (
	"io"
)

//ReadAtReader is io.ReaderAt + io.Reader
type ReadAtReader interface {
	io.ReaderAt
	io.Reader
}

type readAtReader struct {
	cur int64 //we protect underlying reader's position
	io.ReaderAt
}

var _ io.Reader = (*readAtReader)(nil)

//NewReadAtReader returns a reader that using a io.ReaderAt's ReadAt in conjunction
// with its own position counter. This is useful when you have a io.ReaderAt that
// is not also a io.Reader or you want to share an io.ReaderAt that is also an io.Reader
// without modifying its position. For example os.File's position is on the OS/kernel side
// meaning normally sharing it requires locking... or using ReadAt at as we do here.
func NewReadAtReader(rdrAt io.ReaderAt) ReadAtReader {
	return &readAtReader{
		cur:      0,
		ReaderAt: rdrAt,
	}
}

func (rar *readAtReader) Read(buf []byte) (n int, err error) {
	n, err = rar.ReadAt(buf, rar.cur)
	rar.cur += int64(n)
	return n, err
}
