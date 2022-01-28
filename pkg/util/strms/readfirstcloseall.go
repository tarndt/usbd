package strms

import (
	"io"
)

type readFirstCloseList struct {
	io.Reader
	closers []io.Closer
}

var _ io.ReadCloser = readFirstCloseList{}

//NewReadFirstCloseList is a wraper that reads the provided reader but when
// closed will close the provided readers. Useful when you have io.Readers
// wrapping io.ReadClosers.
func NewReadFirstCloseList(rdr io.Reader, closers ...io.Closer) io.ReadCloser {
	return readFirstCloseList{
		Reader:  rdr,
		closers: closers,
	}
}

func (rfca readFirstCloseList) Close() (err error) {
	for _, closer := range rfca.closers {
		if clsrErr := closer.Close(); clsrErr != nil && err == nil {
			err = clsrErr
		}
	}
	return err
}
