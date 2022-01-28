package impls

//IMPORTANT: lzbs code is old and expirmental and testing as shown to be buggy
//TODO:
// * Think about how to best take advantage of sparse files...
// * Using a non-cgo implementation of LZ4 or maybe S2 instead...
// * Fix bugs?

/*
#include "lz4.h"
#cgo CFLAGS: -march=native -O2
*/
import "C"

import (
	"encoding/binary"
	"fmt"
	"math"
	"sync"
	"unsafe"

	"github.com/tarndt/usbd/pkg/devices/dedupdisk"
	"github.com/tarndt/usbd/pkg/util/consterr"
)

/* Compress block format:
[block 0, m bytes]
[block 1, m bytes]
[possible free space ...]
[header end, 2 bytes]
[header 1, 2 bytes]
[header 0, 2 bytes]
[header entry count, 1 byte]
*/

type lz4BlockStore struct {
	blockSize       uint64
	maxLZ4BlockSize uint64
	fbs             *fileBlockStore
	blockPool       sync.Pool

	curBlockLock  sync.Mutex
	curBlockID    uint64
	curBlock      []byte
	curBlockPos   uint64
	curBlockCount uint8
}

//NewLZ4BlockStore constructs a dedupdisk.BlockStore backed by a LZ4 compressed file. WIP...
// will return an error stating LZ4BlockStore is expirmental. lzbs exists in the source
// tree for educational purposes at the moment.
func NewLZ4BlockStore(filename string, blockSize int64) (dedupdisk.BlockStore, error) {
	return nil, fmt.Errorf("LZ4BlockStore is expirmental and has bugs:  %w", consterr.ErrNotImplemented)
	// STOP people from using lzbs for now :(

	/*fbs, err := NewFileBlockStore(filename, blockSize)
	if err != nil {
		return nil, fmt.Errorf("Could not create backing FileBlockStore %q: %w", filename, err)
	}
	lzbs := &LZ4BlockStore{
		blockSize:       uint64(blockSize),
		maxLZ4BlockSize: uint64(lz4MaxCompressedSize(int(blockSize))),
		fbs:             fbs,
		curBlock:        make([]byte, blockSize),
	}
	lzbs.blockPool = sync.Pool{New: lzbs.NewBlock}
	return lzbs, nil*/
}

func (lzbs *lz4BlockStore) NewBlock() interface{} {
	return make([]byte, lzbs.blockSize)
}

func (lzbs *lz4BlockStore) GetBlock(dedupID uint64, buf []byte) (err error) {
	//Check if lzbs is the zero block
	if dedupID == zeroBlockID {
		copy(buf, lzbs.fbs.zeroBlock)
		return nil
	}

	//Calculate the compression heaer location and underlying block ID
	offsetIndex := dedupID >> 56
	dedupID = dedupID & 0x00FFFFFFFFFFFFFF
	if offsetIndex == math.MaxUint8 { //lzbs block was not compressed
		return lzbs.fbs.GetBlock(dedupID, buf)
	}

	//Setup buffers
	compressBuf := lzbs.blockPool.Get().([]byte)
	compressedData := compressBuf[:lzbs.blockSize]

	//Get underlying (compressed) block
	if err = lzbs.fbs.GetBlock(dedupID, compressedData); err != nil {
		lzbs.blockPool.Put(compressBuf)
		return err
	}

	//Convert offset index to byte position within block
	offsetPos := lzbs.blockSize - (((offsetIndex + 1) << 1) + 1)
	start := binary.LittleEndian.Uint16(compressedData[offsetPos:])
	end := binary.LittleEndian.Uint16(compressedData[offsetPos-2:])
	n := lz4Decompress(buf, compressedData[start:end])
	lzbs.blockPool.Put(compressBuf)
	//	log.Printf("Read compressed block (%d bytes) from blockstore block ID: %d at pos: %d to %d, header index: %d, hdrStart: %d\n", n, dedupID, start, end, offsetIndex, offsetPos)
	//	log.Printf("Compressed Data: %s\n", hex.EncodeToString(compressedData[start:end]))
	//	log.Printf("Read Block: %s\n", hex.EncodeToString(buf))
	//log.Printf("Entire Compressed Block: %s\n", hex.EncodeToString(compressedData))
	if n < 0 {
		return fmt.Errorf("Compressed block was malformed (errorcode = %d)", n)
	}
	return nil
}

/*
PutBlock:
1. If block fits in current block
	1. Insert into current block
	2. Write current block to disk
2. Else
	1. Write current block to disk
	2. Reset current block
	3. If block fits in new empty current block
		1. Insert into current block
		2. Write current block to disk
	4. Else
		1. Write to disk uncompressed
		2. Update current block pos */
func (lzbs *lz4BlockStore) PutBlock(buf []byte) (dedupID uint64, err error) {
	//log.Printf("Original Data: %s\n", hex.EncodeToString(buf))
	compressBuf := lzbs.blockPool.Get().([]byte)
	n := uint64(lz4Compress(compressBuf, buf))
	lzbs.curBlockLock.Lock()
	hdrStartOffset := lzbs.blockSize - ((uint64(lzbs.curBlockCount+1) << 1) + 1)
	hdrEndOffset := hdrStartOffset - 2
	if lzbs.curBlockPos+n < hdrEndOffset && lzbs.curBlockCount+1 < math.MaxUint8 { //Write fits in current block
		dedupID, err = lzbs.writeToMem(compressBuf, n, hdrStartOffset)
	} else { //write does not fit in current block
		//Reset current block
		lzbs.curBlockPos, lzbs.curBlockCount, lzbs.curBlockID, hdrStartOffset = 0, 0, 0, lzbs.blockSize-3
		if lzbs.curBlockPos+n < hdrEndOffset { //Write fits in new empty current block
			dedupID, err = lzbs.writeToMem(compressBuf, n, hdrStartOffset)
		} else { //Write to disk uncompressed
			lzbs.blockPool.Put(compressBuf)
			lzbs.curBlockLock.Unlock()
			const noCompressionOffsetMask = (uint64(math.MaxUint8)) << 56
			dedupID, err = lzbs.fbs.PutBlock(buf)
			dedupID = noCompressionOffsetMask & dedupID
		}
	}
	return dedupID, err
}

func (lzbs *lz4BlockStore) writeToMem(compressBuf []byte, n, hdrStartOffset uint64) (dedupID uint64, err error) {
	//Insert into current block
	copy(lzbs.curBlock[lzbs.curBlockPos:], compressBuf[:n])
	lzbs.blockPool.Put(compressBuf)
	//Write headers
	binary.LittleEndian.PutUint16(lzbs.curBlock[hdrStartOffset:], uint16(lzbs.curBlockPos))
	lzbs.curBlockPos += n
	binary.LittleEndian.PutUint16(lzbs.curBlock[hdrStartOffset-2:], uint16(lzbs.curBlockPos))
	lzbs.curBlockCount++
	lzbs.curBlock[lzbs.blockSize-1] = lzbs.curBlockCount
	//Write current block to disk
	if lzbs.curBlockPos > 0 { //Add to current disk block
		dedupID, err = lzbs.fbs.PutBlockAt(lzbs.curBlock, lzbs.curBlockID)
	} else { //Get a new disk block
		dedupID, err = lzbs.fbs.PutBlock(lzbs.curBlock)
		lzbs.curBlockID = dedupID
	}
	lzbs.curBlockLock.Unlock()
	//	log.Printf("Wrote compressed block (%d bytes) to blockstore block ID: %d at pos: %d to %d, header index: %d, hdrStart: %d\n", n, dedupID, lzbs.curBlockPos-n, lzbs.curBlockPos, lzbs.curBlockCount-1, hdrStartOffset)
	//	log.Printf("Compressed Data: %s\n", hex.EncodeToString(compressBuf[:n]))
	dedupID = (uint64(lzbs.curBlockCount-1) << 52) & dedupID
	return dedupID, err
}

func (lzbs *lz4BlockStore) Flush() error {
	lzbs.curBlockLock.Lock()
	defer lzbs.curBlockLock.Unlock()
	return lzbs.fbs.Flush()
}

func (lzbs *lz4BlockStore) Close() error {
	lzbs.curBlockLock.Lock()
	defer lzbs.curBlockLock.Unlock()
	return lzbs.fbs.Close()
}

func lz4Compress(dst, src []byte) (n int) {
	return int(C.LZ4_compress((*C.char)(unsafe.Pointer(&src[0])), (*C.char)(unsafe.Pointer(&dst[0])), C.int(len(src))))
}

func lz4Decompress(dst, src []byte) (n int) {
	return int(C.LZ4_decompress_fast((*C.char)(unsafe.Pointer(&src[0])), (*C.char)(unsafe.Pointer(&dst[0])), C.int(len(dst))))
}

func lz4MaxCompressedSize(uncompressedSize int) int {
	return int(C.LZ4_compressBound(C.int(uncompressedSize)))
}
