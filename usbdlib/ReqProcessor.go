package usbdlib

import (
	"io"
	"log"
	"runtime"
	"sync"
)

//ReqProcessor reqs requests from the NBD command stream (an io.ReadWriter, but
// typically a *NbdStream) exectutes them against the provided Device implementation
// and then writes responses back to the command stream.
type ReqProcessor struct {
	cmdStrm           io.ReadWriter
	dev               Device
	reqQueue          chan *Request
	respQueue         chan *Response
	reqPool, respPool sync.Pool
}

func NewReqProcessor(cmdStrm io.ReadWriter, device Device, workerCount int) {
	this := &ReqProcessor{
		cmdStrm:   cmdStrm,
		dev:       device,
		reqQueue:  make(chan *Request, 64),
		respQueue: make(chan *Response, 32),
		reqPool:   sync.Pool{New: NewRequest},
		respPool:  sync.Pool{New: NewResponse},
	}
	go this.readWorker()
	go this.writeWorker()
	if workerCount < 1 {
		workerCount = RecommendWorkerCount()
	}
	if workerCount -= 2; workerCount < 1 {
		workerCount = 1
	}
	for i := 0; i < workerCount; i++ {
		go this.reqWorker()
	}
}

func RecommendWorkerCount() int {
	return runtime.NumCPU() * 6
}

func (this *ReqProcessor) readWorker() {
	var req *Request
	var err error
	for {
		req = this.reqPool.Get().(*Request)
		if err = req.Decode(this.cmdStrm); err != nil {
			log.Fatalf("ReqProcessor::readWorker(): Decode failed; Details: %s", err)
		}
		this.reqQueue <- req
	}
}

func (this *ReqProcessor) reqWorker() {
	blockSize := this.dev.BlockSize()
	var req *Request
	var resp *Response
	for req = range this.reqQueue {
		if req.pos%blockSize != 0 || int64(req.count)%blockSize != 0 {
			log.Fatalln("ReqProcessor::reqWorker(): Assertion failed: Request was not block aligned (pos=%d,len=%d).", req.pos, req.count)
		}
		resp = this.execute(req, this.respPool.Get().(*Response))
		select {
		case this.respQueue <- resp:
			this.reqPool.Put(req)
		default:
			this.reqPool.Put(req)
			this.respQueue <- resp
		}
	}
}

func (this *ReqProcessor) writeWorker() {
	var resp *Response
	var err error
	for resp = range this.respQueue {
		if resp.Write(this.cmdStrm); err != nil {
			log.Fatalf("ReqProcessor::writeWorker(): Respone reply failed; Details: %s", err)
		}
		this.respPool.Put(resp)
	}
}

func (this *ReqProcessor) execute(req *Request, resp *Response) *Response {
	if resp == nil {
		resp = new(Response)
	}
	var err error
	switch req.reqType {
	case nbdRead:
		_, err = this.dev.ReadAt(resp.GetReadBuffer(req), req.pos)
	case nbdWrite:
		_, err = this.dev.WriteAt(req.writeBuffer, req.pos)
	case nbdTrim:
		err = this.dev.Trim(req.pos, req.count)
	case nbdDiscard:
		err = this.dev.Close()
	default:
		log.Fatalf("ReqProcessor::execute(): Assertion failed: Unknown request type: %d.", req.reqType)
	}
	if err != nil {
		log.Printf("*** WARNING: Request(type = %d, pos = %d, count = %d) failed; Details: %s", req.reqType, req.pos, req.count, err)
	}
	resp.Set(req, err)
	return resp
}
