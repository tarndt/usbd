package objstore

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tarndt/sema"
	"github.com/tarndt/usbd/pkg/devices/objstore/compress"
	"github.com/tarndt/usbd/pkg/devices/objstore/encrypt"
	"github.com/tarndt/usbd/pkg/usbdlib"

	"github.com/dustin/go-humanize"
	"github.com/graymeta/stow"
)

type device struct {
	usbdlib.DefaultBlockSize

	ctx                      context.Context
	ctxCancel                context.CancelFunc
	container                stow.Container
	totalBytes, segmentBytes int64
	segments                 []segment
	quotaSegSema             sema.CountingSema
	pendingOpMu              sync.RWMutex

	//options
	autoflush   time.Duration
	concurFlush uint

	compressMode compress.Mode
	quotaBytes   int64

	noMetadata, thickProvision, keepCache bool

	encryptMode encrypt.Mode
	encryptKey  []byte
}

var _ = (*device)(nil)

//NewDevice is the constructor for ObjectStore backed devices
func NewDevice(ctx context.Context, container stow.Container, cacheDir string, totalBytes, objectBytes uint, options ...Option) (usbdlib.Device, error) {
	switch {
	case ctx == nil:
		return nil, fmt.Errorf("Provided context was nil")
	case container == nil:
		return nil, fmt.Errorf("Provided container was nil")
	case objectBytes > totalBytes:
		return nil, fmt.Errorf("Provided object size (%s) must be not exceeds provided total size (%s)", humanize.IBytes(uint64(objectBytes)), humanize.IBytes(uint64(totalBytes)))
	}

	ctx, cancel := context.WithCancel(ctx)
	dev := &device{
		ctx:          ctx,
		ctxCancel:    cancel,
		container:    container,
		totalBytes:   int64(totalBytes),
		segmentBytes: int64(objectBytes),
	}

	err := dev.applyOpts(options...)
	if err != nil {
		return nil, fmt.Errorf("Could not apply configuration options: %w", err)
	}

	dev.segments, err = loadSegments(dev.container, cacheDir, dev.totalBytes, dev.segmentBytes, dev.thickProvision, dev.quotaSegSema)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("Could not create device using %s: %w", describeContainer(dev.container), err)
	}

	return dev, nil
}

func (dev *device) applyOpts(options ...Option) error {
	for _, opt := range options {
		opt.apply(dev)
	}

	if dev.noMetadata {
		if dev.compressMode != compress.ModeIdentity {
			return fmt.Errorf("Backing store does not support metadata, but %s store compression is enabled", dev.compressMode)
		} else if dev.encryptMode != encrypt.ModeIdentity {
			return fmt.Errorf("Backing store does not support metadata, but %s store encryption is enabled", dev.encryptMode)
		}
	} else {
		if dev.encryptMode != encrypt.ModeIdentity {
			if len(dev.encryptKey) < 1 {
				return fmt.Errorf("%s store encryption is enabled but no key was provided", dev.encryptMode)
			}
			if err := encrypt.ValidAESKey(dev.encryptKey); err != nil {
				return fmt.Errorf("Invalid AES key provided: %w", err)
			}
			dev.container = encrypt.NewEncryptedContainer(dev.container, dev.encryptMode, dev.encryptKey)
		}
		dev.container = compress.NewCompressedContainer(dev.container, dev.compressMode)
	}

	if dev.quotaBytes > 0 {
		if dev.quotaBytes < dev.segmentBytes {
			return fmt.Errorf("Provided quota (%d bytes) was smaller than a single segment/object (%d bytes)", dev.quotaBytes, dev.segmentBytes)
		}
		if dev.quotaBytes < dev.totalBytes {
			dev.quotaSegSema = sema.NewChanSemaTimeout(uint(dev.quotaBytes/dev.segmentBytes), 0)
		}
	}

	if dev.concurFlush < 1 {
		dev.concurFlush = 1
	}

	if dev.autoflush > 0 {
		go dev.flushWorker()
	}

	return nil
}

//Size of this device in bytes
func (dev *device) Size() int64 {
	return dev.totalBytes
}

//ReadAt fufills io.ReaderAt and in turn part of usbdlib.Device
func (dev *device) ReadAt(buf []byte, pos int64) (count int, err error) {
	return dev.ioAt(true, buf, pos)
}

//WriteAt fufills io.WriterAt and in turn part of usbdlib.Device
func (dev *device) WriteAt(buf []byte, pos int64) (count int, err error) {
	return dev.ioAt(false, buf, pos)
}

func (dev *device) ioAt(readOp bool, buf []byte, pos int64) (count int, err error) {
	dev.pendingOpMu.RLock()
	defer dev.pendingOpMu.RUnlock()
	if err := dev.ctx.Err(); err != nil {
		return 0, fmt.Errorf("Device is shutdown: %w", err)
	}

	if pos > dev.totalBytes {
		return 0, io.EOF
	}

	segID, maxSegID := pos/dev.segmentBytes, int64(len(dev.segments))
	pos -= segID * dev.segmentBytes

	totalRead := 0
	remaining := len(buf)

	for remaining > 0 {
		if segID >= maxSegID {
			return totalRead, io.EOF
		}

		segBuf := buf[totalRead:]
		if maxWrite := int(dev.segmentBytes - pos); remaining > maxWrite {
			segBuf = segBuf[:maxWrite]
		}

		const maxCapTries = 10
		capTries := 0
		for {
			if readOp {
				count, err = dev.segments[segID].ReadAt(segBuf, pos)
			} else {
				count, err = dev.segments[segID].WriteAt(segBuf, pos)
			}
			if err != nil {
				if errors.Is(err, errCapacityClaim) && capTries < maxCapTries {
					if _, err = dev.removeLeastRecentUsed(int(segID)); err != nil {
						return count, fmt.Errorf("Could not free capacity to load segment %d: %w", segID, err)
					}
					capTries++
					continue
				}
				return count, fmt.Errorf("Could not access segment %d: %w", segID, err)
			}
			break
		}

		remaining -= count
		totalRead += count
		segID++
		pos = 0
	}

	return totalRead, nil
}

//Trim fufills part of usbdlib.Device
func (dev *device) Trim(pos int64, count int) error {
	return nil //See TODO
}

//Flush fufills part of usbdlib.Device
func (dev *device) Flush() error {
	dev.pendingOpMu.RLock()
	defer dev.pendingOpMu.RUnlock()
	if err := dev.ctx.Err(); err != nil {
		return fmt.Errorf("Device is shutdown: %w", err)
	}

	return dev.flush()
}

func (dev *device) flush() error {
	concurFlush := dev.concurFlush
	if concurFlush < 1 {
		concurFlush = 1
	}

	flushList := make([]*segment, 0, len(dev.segments))
	for i := range dev.segments {
		seg := &dev.segments[i]
		if seg.Dirty() {
			flushList = append(flushList, seg)
		}
	}
	if len(flushList) < 1 {
		return nil
	}

	const maxFlushAlloc = 512 * 1024 * 1024 //512 MB
	flushAlloc := int64(0)
	bufCh := make(chan *bytes.Buffer, concurFlush)
	getBuf := func() *bytes.Buffer {
		select {
		case buf := <-bufCh:
			return buf
		default:
		}

		cur := atomic.AddInt64(&flushAlloc, dev.segmentBytes)
		if cur > maxFlushAlloc {
			return nil
		}
		defer func() {
			recover() //if alloc fails, just abort which returns nil
		}()
		return bytes.NewBuffer(make([]byte, 0, int(dev.segmentBytes)))
	}
	freeBuf := func(buf *bytes.Buffer) {
		if buf == nil {
			return
		}
		buf.Reset()
		bufCh <- buf
	}

	errCh := make(chan error, 1)
	flushSema := sema.NewChanSemaCount(concurFlush)
	var pending sync.WaitGroup
	for _, seg := range flushList {
		if !seg.Dirty() {
			continue
		}
		flushSema.P()
		pending.Add(1)

		go func(s *segment) {
			defer func() {
				flushSema.V()
				pending.Done()
			}()

			buf := getBuf()
			defer freeBuf(buf)

			if err := s.Flush(buf); err != nil {
				select {
				case errCh <- err:
				default:
				}
			}
		}(seg)
	}
	pending.Wait()
	close(errCh)

	if err := <-errCh; err != nil {
		return fmt.Errorf("One ore more segments failed to flush: %w", err)
	}
	return nil
}

//ReadAt fufills io.Closer and in turn part of usbdlib.Device
func (dev *device) Close() error {
	dev.pendingOpMu.Lock()
	defer dev.pendingOpMu.Unlock()
	if err := dev.ctx.Err(); err != nil {
		return nil
	}
	dev.ctxCancel()

	err := dev.flush()
	if err != nil {
		return fmt.Errorf("Failed to write local cache back to remote store during shutdown (local cache will be preserved): %w", err)
	}

	if !dev.keepCache {
		for i := range dev.segments {
			if rmErr := dev.segments[i].DeleteFile(); rmErr != nil && err == nil {
				err = rmErr
			}
		}
	}
	if err != nil {
		return fmt.Errorf("Could not delete one or more local cache files during shutdown: %w", err)
	}

	return nil
}

func (dev *device) flushWorker() {
	if dev.autoflush == 0 {
		return
	}

	ticker := time.NewTicker(dev.autoflush)
	for {
		select {
		case <-dev.ctx.Done():
			ticker.Stop()
			return
		case <-ticker.C:
			dev.Flush()
		}
	}
}

func (dev *device) removeLeastRecentUsed(forIdx int) (int, error) {
	const maxTries = 16

	for tries := 0; true; tries++ {
		var (
			cleanCannidate, dirtyCannidate *segment
			cleanLastOp, dirtyLastOp       time.Time
			cleanIdx, dirtyIdx             int
		)

		for i := range dev.segments {
			if i == forIdx {
				continue
			}

			seg := &dev.segments[i]
			if !seg.Backed() {
				continue
			}

			if seg.Dirty() {
				if lastOp := seg.LastOp(); lastOp.Before(dirtyLastOp) || dirtyLastOp.IsZero() {
					dirtyCannidate, dirtyLastOp, dirtyIdx = seg, lastOp, i
				}
			} else {
				if lastOp := seg.LastOp(); lastOp.Before(cleanLastOp) || cleanLastOp.IsZero() {
					cleanCannidate, cleanLastOp, cleanIdx = seg, lastOp, i
				}
			}
		}

		cannidate, lastOp, idx := cleanCannidate, cleanLastOp, cleanIdx
		if cannidate == nil {
			cannidate, lastOp, idx = dirtyCannidate, dirtyLastOp, dirtyIdx
		}

		if cannidate == nil {
			return 0, fmt.Errorf("Could not remove any segments as none are backed")
		} else if cannidate.Backed() && (cannidate.LastOp().Equal(lastOp) || tries+1 >= maxTries) {
			return idx, cannidate.DeleteFile()
		}
	}
	panic("BUG: removeLeastRecentUsed loop never exists")
}
