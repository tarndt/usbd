package strms

import (
	"io"
)

//WriterAtWriter is io.WriterAt + io.Writer
type WriterAtWriter interface {
	io.WriterAt
	io.Writer
}

type writeAtWriter struct {
	cur int64 //we protect underlying writer's position
	io.WriterAt
}

var _ io.Writer = (*writeAtWriter)(nil)

//NewWriteAtWriter returns a writer that using a io.WriterAt's WriteAt in conjunction
// with its own position counter. See NewReadAtReader for a detailed explanation
// of why is this is often useful.
func NewWriteAtWriter(wtrAt io.WriterAt) WriterAtWriter {
	return &writeAtWriter{
		cur:      0,
		WriterAt: wtrAt,
	}
}

func (waw *writeAtWriter) Write(buf []byte) (n int, err error) {
	n, err = waw.WriteAt(buf, waw.cur)
	waw.cur += int64(n)
	return n, err
}
