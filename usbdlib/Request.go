package usbdlib

import (
	"bytes"
	"encoding/binary"
	"io"

	"github.com/tarndt/usbd/errs"
)

type Request struct {
	rawReq []byte
	//Request fields
	reqType uint32
	handle  []byte
	pos     int64
	count   int
	//Resources to execute requests
	writeBuffer []byte
}

func NewRequest() interface{} {
	this := &Request{
		rawReq: newNbdRawReq(),
		handle: newNbdHandle(),
	}
	return this
}

func (this *Request) Decode(strm io.Reader) error {
	_, err := io.ReadFull(strm, this.rawReq)
	if err != nil {
		return errs.Append(err, "Could not read request data")
	}
	rdr := bytes.NewReader(this.rawReq)
	//Check magic number
	var magicNumber uint32
	if err = binary.Read(rdr, binary.LittleEndian, &magicNumber); err != nil { //TODO use binary.LittleEndian to do this once and cache []bytes
		return err
	} else if magicNumber != nbdReqMagic {
		return errs.Append(err, "Request did not have correct magic number")
	}
	//Read request type
	if err = binary.Read(rdr, binary.BigEndian, &this.reqType); err != nil {
		return errs.Append(err, "Could not decode request type")
	}
	//Read handle
	if _, err = io.ReadFull(rdr, this.handle); err != nil {
		return errs.Append(err, "Could not read file handle")
	}
	//Read pos
	var pos uint64
	if err = binary.Read(rdr, binary.BigEndian, &pos); err != nil {
		return errs.Append(err, "Could not decode position")
	}
	this.pos = int64(pos)
	//Read count
	var count uint32
	if err = binary.Read(rdr, binary.BigEndian, &count); err != nil {
		return errs.Append(err, "Could not decode count")
	}
	this.count = int(count)
	//Do we need to read our data to be written?
	if this.reqType == nbdWrite {
		if _, err = io.ReadFull(strm, this.getWriteBuffer()); err != nil {
			return errs.Append(err, "Could not read data to be written")
		}
	}
	return nil
}

func (this *Request) getWriteBuffer() []byte {
	if cap(this.writeBuffer) < this.count {
		this.writeBuffer = make([]byte, this.count)
	} else {
		this.writeBuffer = this.writeBuffer[:this.count]
	}
	return this.writeBuffer
}
