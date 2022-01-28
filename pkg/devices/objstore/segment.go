package objstore

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tarndt/sema"
	"github.com/tarndt/usbd/pkg/util"

	"github.com/graymeta/stow"
)

type segment struct {
	ID int
	*storeParams

	localFile *os.File
	fileMu    sync.RWMutex

	remoteItem stow.Item
	itemMu     sync.RWMutex

	atomicDirty, atomicBacked                       uint64
	atomicLastReadUnixNano, atomicLastWriteUnixNano int64
}

//loadSegments is a helper that constructs a slice of segments from items stored
// in a remote object store containers that will be cached on the local file system
// * container stow.Container -> where items are presisted remotely
// * cacheDir string          -> local directory files caching remote data
// * {total/segment}Bytes int64 -> bytes per segment and sum of segments
// * thickProvision -> no not use Linux sparse files
// * quotaSema -> counting semaphore for tracking number of segments cached locally
func loadSegments(container stow.Container, cacheDir string, totalBytes, segmentBytes int64, thickProvision bool, quotaSema sema.CountingSema) ([]segment, error) {
	params := &storeParams{container: container, segmentBytes: segmentBytes, cacheDir: cacheDir, thickProvision: thickProvision, quotaSema: quotaSema}
	prefix := osbdPrefix + devicePrefix + container.Name() + blockPrefix
	count := int(totalBytes / segmentBytes)

	segments := make([]segment, count)
	for i := range segments {
		seg := &segments[i]
		seg.ID = i
		seg.storeParams = params
	}

	items, cursor, err := container.Items(prefix, stow.CursorStart, count+1)
	switch {
	case err != nil:
		return nil, fmt.Errorf("Could not enumerate items in %s: %w", describeContainer(container), err)
	case len(items) > count:
		return nil, fmt.Errorf("Enumeration of %s revealed more than the expected %d items", describeContainer(container), count)
	case !stow.IsCursorEnd(cursor):
		return nil, fmt.Errorf("After enumerating the expected %d items the %s indicated more items exist", count, describeContainer(container))
	}

	for _, item := range items {
		segIDStr := strings.TrimPrefix(item.Name(), prefix)             //Remove <osbd-dev_r4xvnq-blk_>X
		segIDStr = strings.TrimSuffix(segIDStr, filepath.Ext(segIDStr)) //Remove any file extension
		segID, err := strconv.Atoi(segIDStr)
		switch {
		case err != nil:
			return nil, fmt.Errorf("Could not parse %s name into block ID while iterating on items in %s: Parse %q failed: %w", describeItem(item), describeContainer(container), segIDStr, err)
		case segID < 0 || segID > count:
			return nil, fmt.Errorf("Parsed ID: %d from %s in %s was not in expected range (0 <= %d <= %d)", segID, describeItem(item), describeContainer(container), segID, count)
		}

		segments[segID].remoteItem = item
	}

	return segments, nil
}

//ReadAt reads len(buf) bytes from the segment starting at byte offset pos (position).
// It returns the number of bytes read and an error, if any.
func (seg *segment) ReadAt(buf []byte, pos int64) (count int, err error) {
	atomic.StoreInt64(&seg.atomicLastReadUnixNano, time.Now().UnixNano())

	data, unlock, err := seg.loadFile(false)
	if err != nil {
		return 0, fmt.Errorf("Could not load segment data for reading: %w", err)
	}

	//Section is empty, return as as many zeros as min(len(buf), seg bytes after pos)
	if data == nil {
		unlock()
		bufLen := len(buf)
		if end := pos + int64(bufLen); end > seg.segmentBytes {
			bufLen, err = int(seg.segmentBytes-pos), io.EOF
		}

		util.ZeroFill(buf[:bufLen])
		return bufLen, err
	}
	defer unlock()

	return data.ReadAt(buf, pos)
}

//WriteAt writes len(buf) bytes to the segment starting at byte offset pos (position).
// It returns the number of bytes written and an error, if any.
func (seg *segment) WriteAt(buf []byte, pos int64) (count int, err error) {
	atomic.StoreInt64(&seg.atomicLastWriteUnixNano, time.Now().UnixNano())

	if util.IsZeros(buf) { //if write is all zeros and segment is empty write is a noop
		seg.fileMu.RLock()
		data := seg.localFile
		seg.fileMu.RUnlock()
		if data == nil {
			return len(buf), nil
		}
	}

	data, unlock, err := seg.loadFile(true)
	if err != nil {
		return 0, fmt.Errorf("Could not load segment data for writing: %w", err)
	}
	defer unlock()

	count, err = data.WriteAt(buf, pos)
	if count > 0 {
		atomic.StoreUint64(&seg.atomicDirty, 1)
	}
	return count, err
}

//loadFile is a helper that returns the local data, file creating it if needed and desired and returns the file's corresponding access mutex
func (seg *segment) loadFile(createWrite bool) (file *os.File, unlock func(), err error) {
	if !createWrite {
		seg.fileMu.RLock()
		if seg.localFile != nil {
			return seg.localFile, seg.fileMu.RUnlock, nil
		}
		seg.fileMu.RUnlock()
	}

	seg.fileMu.Lock()
	if seg.localFile != nil {
		return seg.localFile, seg.fileMu.Unlock, nil
	}

	seg.itemMu.RLock()
	if seg.remoteItem == nil {
		seg.itemMu.RUnlock()
		if createWrite {
			seg.localFile, err = seg.storeParams.createFile(strconv.Itoa(seg.ID))
		} else {
			return nil, seg.fileMu.Unlock, nil
		}
	} else {
		defer seg.itemMu.RUnlock()
		seg.localFile, err = seg.storeParams.downloadFile(seg.remoteItem)
	}

	if err != nil {
		seg.fileMu.Unlock()
		return nil, nil, err
	}
	atomic.StoreUint64(&seg.atomicBacked, 1)
	return seg.localFile, seg.fileMu.Unlock, nil
}

//Flush will persist a dirty segment to the backing store. If a optional buffer
// is provided the segment will be copied into memory and unblock writes to proceed
// concurrently with any upload
func (seg *segment) Flush(optBuf *bytes.Buffer) error {
	if !seg.Dirty() {
		return nil
	}

	seg.fileMu.RLock()
	return seg.flush(optBuf, seg.fileMu.RUnlock)
}

//flush is the internal flush implementation, the unlock method of the file.(R)Lock acquired
// can be passed to allow buffered uploads to unblock early
func (seg *segment) flush(optBuf *bytes.Buffer, optEarlyUnlock func()) (err error) {
	if seg.localFile == nil {
		if optEarlyUnlock != nil {
			optEarlyUnlock()
		}
		return fmt.Errorf("Segment found to be dirty during flush request but is missing a backing data file")
	}

	seg.itemMu.Lock()
	defer seg.itemMu.Unlock()

	atomic.StoreUint64(&seg.atomicDirty, 0) //mark clean so if somone writes during upload we revert to dirty
	seg.remoteItem, err = seg.storeParams.syncFile(seg.localFile, strconv.Itoa(seg.ID), optBuf, optEarlyUnlock)

	return err
}

//DeleteFile persists the segment to remote storage and removes the local data
func (seg *segment) DeleteFile() (err error) {
	seg.fileMu.Lock()
	defer seg.fileMu.Unlock()

	if !seg.Backed() {
		return nil
	}

	if seg.Dirty() {
		if err = seg.flush(nil, nil); err != nil {
			return fmt.Errorf("Flush during DeleteFile failed: %w", err)
		}
	}

	filename := seg.localFile.Name()
	if err = seg.localFile.Close(); err != nil {
		return fmt.Errorf("Could not close local cache file %q before deleting it: %w", filename, err)
	}

	if err = os.Remove(filename); err != nil {
		return fmt.Errorf("Could not remove local file %q: %w", filename, err)
	}
	atomic.StoreUint64(&seg.atomicBacked, 0)
	seg.localFile = nil
	seg.releaseCapacity()

	return nil
}

//Drity returns if the segment has local writes not committed to the remote store
func (seg *segment) Dirty() bool {
	return atomic.LoadUint64(&seg.atomicDirty) == 1
}

//Drity returns if the segment has a locally stored data
func (seg *segment) Backed() bool {
	return atomic.LoadUint64(&seg.atomicBacked) == 1
}

//LastOp returns the last time a read or write operation took place on this segment
func (seg *segment) LastOp() time.Time {
	lastWrite, lastRead := seg.LastWrite(), seg.LastRead()
	if lastRead.Before(lastWrite) {
		return lastRead
	}
	return lastWrite
}

//LastRead returns the last time a read operation took place on this segment
func (seg *segment) LastRead() time.Time {
	return atomicLoadTime(&seg.atomicLastReadUnixNano)
}

//LastWrite returns the last time a read operation took place on this segment
func (seg *segment) LastWrite() time.Time {
	return atomicLoadTime(&seg.atomicLastWriteUnixNano)
}

//atomicLoadTime is a helper to atomicly load a nanosecond counter field and
// convert it to a time.Time
func atomicLoadTime(atomicNanos *int64) time.Time {
	nsec := atomic.LoadInt64(atomicNanos)
	if nsec < 1 {
		return time.Time{}
	}
	return time.Unix(0, nsec)
}
