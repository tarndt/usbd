package usbdlib

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"runtime"
	"sync"
)

//RecommendWorkerCount returns a empircally derived heuristic for the optimal
// number of work goroutines on this machine
func RecommendWorkerCount() int {
	return runtime.NumCPU() * 6
}

//ReqProcessor reqs requests from the NBD command stream (an io.ReadWriter, but
// typically a *NbdStream) exectutes them against the provided Device implementation
// and then writes responses back to the command stream.
type reqProcessor struct {
	blockSize         int64
	cmdStrm           io.ReadWriteCloser
	dev               Device
	reqQueue          chan *request
	respQueue         chan *response
	reqPool, respPool sync.Pool

	ctx       context.Context
	ctxCancel context.CancelFunc
	workersWg sync.WaitGroup
}

func processRequests(ctx context.Context, cmdStrm io.ReadWriteCloser, device Device, workerCount int) error {
	ctx, cancel := context.WithCancel(ctx)

	this := &reqProcessor{
		blockSize: device.BlockSize(),
		cmdStrm:   cmdStrm,
		dev:       device,
		reqQueue:  make(chan *request, 64),
		respQueue: make(chan *response, 32),
		reqPool:   sync.Pool{New: newRequest},
		respPool:  sync.Pool{New: newResponse},
		ctx:       ctx,
		ctxCancel: cancel,
	}
	go this.readStrmWorker()
	go this.writeStrmWorker()

	if workerCount < 1 {
		workerCount = RecommendWorkerCount()
	}
	if workerCount -= 2; workerCount < 1 {
		workerCount = 1
	}
	this.workersWg.Add(workerCount)
	for i := 0; i < workerCount; i++ {
		go this.reqIOWorker()
	}

	//Shutdown
	<-ctx.Done()          //readStrmWorker is checking this and will close this.reqQueue killing IO workers
	this.workersWg.Wait() //All IO workers have shutdown
	close(this.respQueue) //Kills writeStrmWorker
	if err := this.dev.Close(); err != nil {
		return fmt.Errorf("Could not close userspace device: %w", err)
	}
	return nil
}

func (proc *reqProcessor) readStrmWorker() {
	defer close(proc.reqQueue)

	bufStrm := bufio.NewReaderSize(proc.cmdStrm, 16*1024*1024)
	var req *request
	var err error
	flushMu := new(sync.RWMutex)
	for {
		if err = proc.ctx.Err(); err != nil {
			return
		}

		req = proc.reqPool.Get().(*request)
		if err = req.Decode(bufStrm); err != nil {
			if proc.ctx.Err() != nil {
				return
			}
			log.Printf("ReqProcessor::readWorker(): Decode failed; Details: %s", err)
			if err = proc.cmdStrm.Close(); err != nil {
				log.Printf("ReqProcessor::readWorker(): Could not close command stream; Details: %s", err)
			}
			proc.ctxCancel()
			return
		}
		switch req.reqType {
		case nbdWrite, nbdTrim:
			flushMu.RLock()
			req.flushMu = flushMu

		case nbdFlush:
			req.flushMu = flushMu
			flushMu = new(sync.RWMutex)

		case nbdDisconnect:
			proc.ctxCancel()
			return
		}
		proc.reqQueue <- req
	}
}

func (proc *reqProcessor) writeStrmWorker() {
	var resp *response
	var open bool
	var err error

	bufStrm := bufio.NewWriterSize(proc.cmdStrm, 16*1024*1024)
	defer bufStrm.Flush()

	for {
		select {
		case resp, open = <-proc.respQueue:
			if !open {
				return
			}

			if resp.Write(proc.cmdStrm); err != nil {
				log.Printf("ReqProcessor::writeWorker(): Response reply failed; Details: %s", err)
				proc.ctxCancel()
			}
			proc.respPool.Put(resp)

		default:
			if err = bufStrm.Flush(); err != nil {
				log.Printf("ReqProcessor::writeWorker(): Flushing buffered replies failed; Details: %s", err)
				proc.ctxCancel()
			}

			select {
			case resp, open = <-proc.respQueue:
				if !open {
					return
				}

				if resp.Write(proc.cmdStrm); err != nil {
					log.Printf("ReqProcessor::writeWorker(): Response reply failed; Details: %s", err)
					proc.ctxCancel()
				}
				proc.respPool.Put(resp)
			}
		}
	}
}

func (proc *reqProcessor) reqIOWorker() {
	defer proc.workersWg.Done()

	var req *request
	var resp *response
	for req = range proc.reqQueue {
		resp = proc.execute(req, proc.respPool.Get().(*response))
		select {
		case proc.respQueue <- resp:
			proc.reqPool.Put(req)
		default:
			proc.reqPool.Put(req)
			proc.respQueue <- resp
		}
	}
}

func (proc *reqProcessor) execute(req *request, resp *response) *response {
	if resp == nil {
		resp = new(response)
	}

	var err error
	var errCode nbdErr

	switch req.reqType {
	case nbdRead:
		if req.pos%proc.blockSize != 0 || int64(req.count)%proc.blockSize != 0 {
			err = fmt.Errorf("Assertion failed: Read request was not block aligned (pos=%d,len=%d)", req.pos, req.count)
			errCode = ndbRespErrInvalid
			break
		}

		_, err = proc.dev.ReadAt(resp.GetReadBuffer(req), req.pos)
		if err != nil {
			errCode = ndbRespErrIO
		}

	case nbdWrite:
		if req.pos%proc.blockSize != 0 || int64(req.count)%proc.blockSize != 0 {
			err = fmt.Errorf("Assertion failed: Write request was not block aligned (pos=%d,len=%d)", req.pos, req.count)
			errCode = ndbRespErrInvalid
			break
		}

		defer req.flushMu.RUnlock()
		_, err = proc.dev.WriteAt(req.writeBuffer, req.pos)
		if err != nil {
			errCode = ndbRespErrIO
		}

	case nbdFlush:
		req.flushMu.Lock()
		req.flushMu.Unlock() //We can release right away, we just need to ensure previous writes finished
		err = proc.dev.Flush()
		if err != nil {
			errCode = ndbRespErrIO
		}

	case nbdTrim:
		defer req.flushMu.RUnlock()
		err = proc.dev.Trim(req.pos, req.count)
		if err != nil {
			errCode = ndbRespErrIO
		}

	default:
		errCode = ndbRespErrInvalid
		err = fmt.Errorf("Assertion failed: Unknown NBD request type: %d", req.reqType)
	}

	if err != nil {
		log.Printf("ReqProcessor::execute(): WARNING: Request(type = %d, pos = %d, count = %d) and will return %s. Failure was: %s", req.reqType, req.pos, req.count, errCode, err)
	}
	resp.Set(req, errCode)
	return resp
}
