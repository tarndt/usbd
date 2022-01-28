package usbdlib

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

//NBD protocol details: https://github.com/NetworkBlockDevice/nbd/blob/master/doc/proto.md#simple-reply-message
type response struct {
	respBuffer *bytes.Buffer
	reqType    uint32
	readBuffer []byte
	errCode    nbdErr
}

func newResponse() interface{} {
	return &response{
		respBuffer: bytes.NewBuffer(newNbdRawResp()[:0]),
	}
}

func (resp *response) Set(req *request, errCode nbdErr) {
	resp.reqType = req.reqType
	resp.errCode = errCode

	//Build response
	resp.respBuffer.Truncate(0)
	binary.Write(resp.respBuffer, binary.LittleEndian, &nbdReplyMagic) //TODO use binary.LittleEndian to do this once and cache []bytes
	binary.Write(resp.respBuffer, binary.BigEndian, &errCode)
	resp.respBuffer.Write(req.handle)
}

func (resp *response) GetReadBuffer(req *request) []byte {
	if cap(resp.readBuffer) < req.count {
		resp.readBuffer = make([]byte, req.count)
	} else {
		resp.readBuffer = resp.readBuffer[:req.count]
	}
	return resp.readBuffer
}

func (resp *response) Write(strm io.Writer) error {
	_, err := resp.respBuffer.WriteTo(strm)
	if err != nil {
		return fmt.Errorf("Could not write response to response stream: %w", err)
	}
	if resp.reqType == nbdRead {
		if _, err = strm.Write(resp.readBuffer); err != nil {
			return fmt.Errorf("Could not write data read to response stream: %w", err)
		}
	}
	return nil
}
