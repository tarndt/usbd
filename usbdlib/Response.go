package usbdlib

import (
	"bytes"
	"encoding/binary"
	"io"

	"usbd/errs"
)

type Response struct {
	respBuffer *bytes.Buffer
	reqType    uint32
	readBuffer []byte
	err        error
}

func NewResponse() interface{} {
	return &Response{
		respBuffer: bytes.NewBuffer(newNbdRawResp()[:0]),
	}
}

func (this *Response) Set(req *Request, err error) {
	this.reqType = req.reqType
	this.err = err
	//Build response
	this.respBuffer.Truncate(0)
	binary.Write(this.respBuffer, binary.LittleEndian, &nbdReplyMagic) //TODO use binary.LittleEndian to do this once and cache []bytes
	var errCode uint32
	if err != nil {
		errCode = 1
	}
	binary.Write(this.respBuffer, binary.BigEndian, &errCode)
	this.respBuffer.Write(req.handle)
}

func (this *Response) GetReadBuffer(req *Request) []byte {
	if cap(this.readBuffer) < req.count {
		this.readBuffer = make([]byte, req.count)
	} else {
		this.readBuffer = this.readBuffer[:req.count]
	}
	return this.readBuffer
}

func (this *Response) Write(strm io.Writer) error {
	_, err := this.respBuffer.WriteTo(strm)
	if err != nil {
		return errs.Append(err, "Could not write response to response stream")
	}
	if this.reqType == nbdRead {
		if _, err = strm.Write(this.readBuffer); err != nil {
			return errs.Append(err, "Could not write data read to response stream")
		}
	}
	return nil
}
