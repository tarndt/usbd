package usbdlib

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"sync"
)

//NBD protocol details: https://github.com/NetworkBlockDevice/nbd/blob/master/doc/proto.md#request-message
type request struct {
	rawReq []byte
	//Request fields
	reqType uint32
	handle  []byte
	pos     int64
	count   int
	//Resources to execute requests
	writeBuffer []byte
	//Flush mutex; all write ops must RLock, flushes Lock to ensure all previous
	//writes are committed before flushing
	flushMu *sync.RWMutex
}

func newRequest() interface{} {
	req := &request{
		rawReq: newNbdRawReq(),
		handle: newNbdHandle(),
	}
	return req
}

func (req *request) Decode(strm io.Reader) error {
	_, err := io.ReadFull(strm, req.rawReq)
	if err != nil {
		return fmt.Errorf("Could not read request data: %w", err)
	}
	rdr := bytes.NewReader(req.rawReq)

	//Check magic number
	var magicNumber uint32
	if err = binary.Read(rdr, binary.LittleEndian, &magicNumber); err != nil { //TODO use binary.LittleEndian to do this once and cache []bytes
		return err
	} else if magicNumber != nbdReqMagic {
		return fmt.Errorf("Request did not have correct magic number: %w", err)
	}

	//Read request type
	if err = binary.Read(rdr, binary.BigEndian, &req.reqType); err != nil {
		return fmt.Errorf("Could not decode request type: %w", err)
	}

	//Read handle
	if _, err = io.ReadFull(rdr, req.handle); err != nil {
		return fmt.Errorf("Could not read operation handle: %w", err)
	}

	//Read pos
	var pos uint64
	if err = binary.Read(rdr, binary.BigEndian, &pos); err != nil {
		return fmt.Errorf("Could not decode position: %w", err)
	}
	req.pos = int64(pos)

	//Read count
	var count uint32
	if err = binary.Read(rdr, binary.BigEndian, &count); err != nil {
		return fmt.Errorf("Could not decode count: %w", err)
	}
	req.count = int(count)

	//Do we need to read our data to be written?
	if req.reqType == nbdWrite {
		if _, err = io.ReadFull(strm, req.getWriteBuffer()); err != nil {
			return fmt.Errorf("Could not read data to be written: %w", err)
		}
	}

	return nil
}

func (req *request) getWriteBuffer() []byte {
	if cap(req.writeBuffer) < req.count {
		req.writeBuffer = make([]byte, req.count)
	} else {
		req.writeBuffer = req.writeBuffer[:req.count]
	}
	return req.writeBuffer
}
