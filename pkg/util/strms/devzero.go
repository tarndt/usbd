package strms

import (
	"bytes"
	"io"
)

//EmptyRdr is a reader that will always be empty
var EmptyRdr io.Reader = bytes.NewReader([]byte{})

//DevZero is like is like /dev/zero for reading and /dev/null for writing
var DevZero devZero

type devZero struct{}

var _ io.ReadWriter = devZero{}

func (devZero) Read(buf []byte) (int, error) {
	for i := range buf {
		buf[i] = 0
	}
	return len(buf), nil
}

func (devZero) Write(buf []byte) (int, error) {
	return len(buf), nil
}
