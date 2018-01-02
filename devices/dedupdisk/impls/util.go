package impls

import (
	"errors"
	"math"
	"reflect"
	"unsafe"
)

const zeroBlockId = math.MaxUint64

var errNotPresent = errors.New("ID is not present in database")

func isZeros(block []byte) bool {
	bytesHdr := *((*reflect.SliceHeader)(unsafe.Pointer(&block)))
	count := bytesHdr.Len / 8
	longsHdr := reflect.SliceHeader{Data: bytesHdr.Data, Len: count, Cap: count}
	longs := *(*[]uint64)(unsafe.Pointer(&longsHdr))
	for i := 0; i < count; i++ {
		if longs[i] != 0 {
			return false
		}
	}
	return true
}
