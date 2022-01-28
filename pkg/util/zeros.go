package util

import ( //Dark magic to make simple things fast...
	"reflect"
	"unsafe"
)

//IsZeros confirms the provided byte slice contains only zeros by returning true
func IsZeros(block []byte) bool {
	const maxAlignedMagic int = 0xFFFFFFF8

	bytesHdr := *((*reflect.SliceHeader)(unsafe.Pointer(&block)))
	alignedCount := (bytesHdr.Len & maxAlignedMagic) / 8
	var remainingStart int

	if alignedCount > 0 {
		longsHdr := reflect.SliceHeader{Data: bytesHdr.Data, Len: alignedCount, Cap: alignedCount}
		longs := *(*[]uint64)(unsafe.Pointer(&longsHdr))
		for _, long := range longs {
			if long != 0 {
				return false
			}
		}
		remainingStart = alignedCount * 8
	}

	for _, char := range block[remainingStart:] {
		if char != 0 {
			return false
		}
	}

	return true
}

//ZeroFill ensures the provided byte slice contains only zeros
func ZeroFill(block []byte) {
	if len(block) < 1 {
		return
	}

	block[0] = 0
	for i := 1; i < len(block); i <<= 1 {
		copy(block[i:], block[:i])
	}
}
