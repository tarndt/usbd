package usbdlib

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"testing"
)

func TestDecode(t *testing.T) {
	var buf bytes.Buffer
	for i := int64(0); i < int64(DefaultBlockSizeBytes*2); i++ {
		in := newRequest().(*request)

		in.count = int(i)
		if i%2 == 0 {
			in.reqType = nbdRead
		} else {
			in.reqType = nbdWrite
			in.writeBuffer = make([]byte, in.count)
			for i := range in.writeBuffer {
				in.writeBuffer[i] = 7
			}
		}

		in.handle = encodeHandle(i)
		in.pos = i

		buf.Reset()
		if err := in.Encode(&buf); err != nil {
			t.Fatalf("Failed to encode request: %s", err)
		}
		in.rawReq = buf.Bytes()[:ndbReqBytes]

		out := newRequest().(*request)
		if err := out.Decode(&buf); err != nil {
			t.Fatalf("Failed to decode request: %s", err)
		}

		switch {
		case !bytes.Equal(in.rawReq, out.rawReq):
			t.Fatalf("Output of encoding and data read from stream did not match:\n\tin:  %v\n\tvs\n\tout: %v", in.rawReq, out.rawReq)
		case in.reqType != out.reqType:
			t.Fatalf("Wrong request type: in %d vs out %d", in.reqType, out.reqType)
		case !bytes.Equal(in.handle, out.handle):
			t.Fatalf("Wrong request handle: in %v vs out %v", in.handle, out.handle)
		case in.pos != out.pos:
			t.Fatalf("Wrong request position: in %d vs out %d", in.pos, out.pos)
		case in.count != out.count:
			t.Fatalf("Wrong request count: in %d vs out %d", in.count, out.count)
		case in.reqType == nbdWrite && !bytes.Equal(in.writeBuffer, out.writeBuffer):
			t.Fatal("Input and output write buffers did not match")
		}
	}
}

func BenchmarkDecode(b *testing.B) {
	in := newRequest().(*request)
	in.reqType = nbdRead
	in.count = int(1)
	in.handle = encodeHandle(1)
	in.pos = 1

	var buf bytes.Buffer
	if err := in.Encode(&buf); err != nil {
		b.Fatalf("Failed to encode request: %s", err)
	}
	rawReq := buf.Bytes()

	req := newRequest().(*request)
	var err error

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if err = req.Decode(bytes.NewBuffer(rawReq)); err != nil {
			b.Fatalf("Failed to decode request: %s", err)
		}
	}
}

func encodeHandle(x int64) []byte {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(x))
	return buf
}

func (req *request) Encode(wtr io.Writer) error {
	//Write magic number
	var err error
	if err = binary.Write(wtr, binary.LittleEndian, nbdReqMagic); err != nil {
		return fmt.Errorf("Could not encode request type: %w", err)
	}

	//Write request type
	if err = binary.Write(wtr, binary.BigEndian, &req.reqType); err != nil {
		return fmt.Errorf("Could not encode request type: %w", err)
	}

	//Write handle
	if _, err = wtr.Write(req.handle); err != nil {
		return fmt.Errorf("Could not write operation handle: %w", err)
	}

	//Write pos
	pos := uint64(req.pos)
	if err = binary.Write(wtr, binary.BigEndian, &pos); err != nil {
		return fmt.Errorf("Could not encode position: %w", err)
	}

	//Write count
	count := uint32(req.count)
	if err = binary.Write(wtr, binary.BigEndian, &count); err != nil {
		return fmt.Errorf("Could not encode count: %w", err)
	}

	if req.reqType == nbdWrite {
		if len(req.writeBuffer) != int(count) {
			return fmt.Errorf("Write count %d did not match size of write buffer %d", count, len(req.writeBuffer))
		}
		if _, err = wtr.Write(req.writeBuffer); err != nil {
			return fmt.Errorf("Failed to write contents of write buffer: %w", err)
		}
	}

	return nil
}
