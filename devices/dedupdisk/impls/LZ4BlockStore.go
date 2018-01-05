package impls

/*
#include "lz4.h"
#cgo CFLAGS: -march=native -O2
*/
import "C"

import (
	"encoding/binary"
	"math"
	"sync"
	"unsafe"

	"github.com/tarndt/usbd/errs"
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

type LZ4BlockStore struct {
	blockSize       uint64
	maxLZ4BlockSize uint64
	fbs             *FileBlockStore
	blockPool       sync.Pool

	curBlockLock  sync.Mutex
	curBlockId    uint64
	curBlock      []byte
	curBlockPos   uint64
	curBlockCount uint8
}

func NewLZ4BlockStore(filename string, blockSize int64) (*LZ4BlockStore, error) {
	fbs, err := NewFileBlockStore(filename, blockSize)
	if err != nil {
		return nil, errs.Append(err, "Could not create backing FileBlockStore")
	}
	this := &LZ4BlockStore{
		blockSize:       uint64(blockSize),
		maxLZ4BlockSize: uint64(lz4MaxCompressedSize(int(blockSize))),
		fbs:             fbs,
		curBlock:        make([]byte, blockSize),
	}
	this.blockPool = sync.Pool{New: this.NewBlock}
	return this, nil
}

func (this *LZ4BlockStore) NewBlock() interface{} {
	return make([]byte, this.blockSize)
}

func (this *LZ4BlockStore) GetBlock(dedupId uint64, buf []byte) (err error) {
	//Check if this is the zero block
	if dedupId == zeroBlockId {
		copy(buf, this.fbs.zeroBlock)
		return nil
	}
	//Calculate the compression heaer location and underlying block ID
	offsetIndex := dedupId >> 56
	dedupId = dedupId & 0x00FFFFFFFFFFFFFF
	if offsetIndex == math.MaxUint8 { //This block was not compressed
		return this.fbs.GetBlock(dedupId, buf)
	}
	//Setup buffers
	compressBuf := this.blockPool.Get().([]byte)
	compressedData := compressBuf[:this.blockSize]
	//Get underlying (compressed) block
	if err = this.fbs.GetBlock(dedupId, compressedData); err != nil {
		this.blockPool.Put(compressBuf)
		return err
	}
	//Convert offset index to byte position within block
	offsetPos := this.blockSize - (((offsetIndex + 1) << 1) + 1)
	start := binary.LittleEndian.Uint16(compressedData[offsetPos:])
	end := binary.LittleEndian.Uint16(compressedData[offsetPos-2:])
	n := lz4Decompress(buf, compressedData[start:end])
	this.blockPool.Put(compressBuf)
	//	log.Printf("Read compressed block (%d bytes) from blockstore block ID: %d at pos: %d to %d, header index: %d, hdrStart: %d\n", n, dedupId, start, end, offsetIndex, offsetPos)
	//	log.Printf("Compressed Data: %s\n", hex.EncodeToString(compressedData[start:end]))
	//	log.Printf("Read Block: %s\n", hex.EncodeToString(buf))
	//log.Printf("Entire Compressed Block: %s\n", hex.EncodeToString(compressedData))
	if n < 0 {
		return errs.New("Compressed block was malformed (errorcode = %d)", n)
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
func (this *LZ4BlockStore) PutBlock(buf []byte) (dedupId uint64, err error) {
	//log.Printf("Original Data: %s\n", hex.EncodeToString(buf))
	compressBuf := this.blockPool.Get().([]byte)
	n := uint64(lz4Compress(compressBuf, buf))
	this.curBlockLock.Lock()
	hdrStartOffset := this.blockSize - ((uint64(this.curBlockCount+1) << 1) + 1)
	hdrEndOffset := hdrStartOffset - 2
	if this.curBlockPos+n < hdrEndOffset && this.curBlockCount+1 < math.MaxUint8 { //Write fits in current block
		dedupId, err = this.writeToMem(compressBuf, n, hdrStartOffset)
	} else { //write does not fit in current block
		//Reset current block
		this.curBlockPos, this.curBlockCount, this.curBlockId, hdrStartOffset = 0, 0, 0, this.blockSize-3
		if this.curBlockPos+n < hdrEndOffset { //Write fits in new empty current block
			dedupId, err = this.writeToMem(compressBuf, n, hdrStartOffset)
		} else { //Write to disk uncompressed
			this.blockPool.Put(compressBuf)
			this.curBlockLock.Unlock()
			const noCompressionOffsetMask = (uint64(math.MaxUint8)) << 56
			dedupId, err = this.fbs.PutBlock(buf)
			dedupId = noCompressionOffsetMask & dedupId
		}
	}
	return dedupId, err
}

func (this *LZ4BlockStore) writeToMem(compressBuf []byte, n, hdrStartOffset uint64) (dedupId uint64, err error) {
	//Insert into current block
	copy(this.curBlock[this.curBlockPos:], compressBuf[:n])
	this.blockPool.Put(compressBuf)
	//Write headers
	binary.LittleEndian.PutUint16(this.curBlock[hdrStartOffset:], uint16(this.curBlockPos))
	this.curBlockPos += n
	binary.LittleEndian.PutUint16(this.curBlock[hdrStartOffset-2:], uint16(this.curBlockPos))
	this.curBlockCount++
	this.curBlock[this.blockSize-1] = this.curBlockCount
	//Write current block to disk
	if this.curBlockPos > 0 { //Add to current disk block
		dedupId, err = this.fbs.PutBlockAt(this.curBlock, this.curBlockId)
	} else { //Get a new disk block
		dedupId, err = this.fbs.PutBlock(this.curBlock)
		this.curBlockId = dedupId
	}
	this.curBlockLock.Unlock()
	//	log.Printf("Wrote compressed block (%d bytes) to blockstore block ID: %d at pos: %d to %d, header index: %d, hdrStart: %d\n", n, dedupId, this.curBlockPos-n, this.curBlockPos, this.curBlockCount-1, hdrStartOffset)
	//	log.Printf("Compressed Data: %s\n", hex.EncodeToString(compressBuf[:n]))
	dedupId = (uint64(this.curBlockCount-1) << 52) & dedupId
	return dedupId, err
}

func (this *LZ4BlockStore) Flush() error {
	this.curBlockLock.Lock()
	defer this.curBlockLock.Unlock()
	return this.fbs.Flush()
}

func (this *LZ4BlockStore) Close() error {
	this.curBlockLock.Lock()
	defer this.curBlockLock.Unlock()
	return this.fbs.Close()
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
