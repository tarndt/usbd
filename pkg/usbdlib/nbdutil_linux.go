package usbdlib

import (
	"fmt"
)

/*
#include <linux/ioctl.h> //needed for _IO macro in nbd.h
#include <linux/nbd.h> //struct nbd_request, struct nbd_reply
#include <unistd.h> //size_t
#include <netinet/in.h> //htonl

const size_t sizeofNdbReq = sizeof(struct nbd_request);
const size_t sizeofNdbResp = sizeof(struct nbd_reply);
struct nbd_reply dummyReply;
const int ndbHandleLen = sizeof(dummyReply.handle);


#if defined NBD_SET_FLAGS && defined NBD_FLAG_SEND_TRIM
	const int ndbTrimSupported = 1;
#else
	const int ndbTrimSupported = 0;
#endif

*/
import "C"

const (
	//ioctl constants
	nbdSetBlockSize  = uintptr(C.NBD_SET_BLKSIZE)
	nbdSetSizeBlocks = uintptr(C.NBD_SET_SIZE_BLOCKS)
	ndbClearSock     = uintptr(C.NBD_CLEAR_SOCK)
	nbdSetSock       = uintptr(C.NBD_SET_SOCK)
	nbdSetFlags      = uintptr(C.NBD_SET_FLAGS)
	nbdFlagSendTrim  = uintptr(C.NBD_FLAG_SEND_TRIM)
	nbdDoIt          = uintptr(C.NBD_DO_IT)
	nbdClearQueue    = uintptr(C.NBD_CLEAR_QUE)
	ndbDisconnect    = uintptr(C.NBD_DISCONNECT)

	//NBD commands
	nbdRead       = uint32(C.NBD_CMD_READ)
	nbdWrite      = uint32(C.NBD_CMD_WRITE)
	nbdDisconnect = uint32(C.NBD_CMD_DISC)
	nbdFlush      = uint32(C.NBD_CMD_FLUSH)
	nbdTrim       = uint32(C.NBD_CMD_TRIM)

	//NBD response error numbers (#defines in nbd.h)
	nbdRespSuccess          = nbdErr(0)
	ndbRespErrPerms         = nbdErr(1)
	ndbRespErrIO            = nbdErr(5)
	ndbRespErrMem           = nbdErr(12)
	ndbRespErrInvalid       = nbdErr(22)
	ndbRespErrNoSpace       = nbdErr(28)
	ndbRespErrTooLarge      = nbdErr(75)
	ndbRespErrUnsupportedOp = nbdErr(95)
	ndbRespErrShuttingDown  = nbdErr(108)
)

type nbdErr uint32

func newRespErr(errNo uint32) error {
	if errNo == 0 {
		return nil
	}
	return nbdErr(errNo)
}

func (err nbdErr) Error() string {
	var msg = fmt.Sprintf("NBD Error #%d: ", err)
	switch err {
	case nbdRespSuccess:
		msg = "Not an error!"
	case ndbRespErrPerms:
		msg += "Operation not permitted"
	case ndbRespErrIO:
		msg += "Input/output error"
	case ndbRespErrMem:
		msg += "Cannot allocate memory"
	case ndbRespErrInvalid:
		msg += "Invalid argument"
	case ndbRespErrNoSpace:
		msg += "No space left on device"
	case ndbRespErrTooLarge:
		msg += "Value too large"
	case ndbRespErrUnsupportedOp:
		msg += "Operation not supported"
	case ndbRespErrShuttingDown:
		msg += "Server is in the process of being shut down"
	default:
		msg += "Unknown (likely invalid) error"
	}
	return msg
}

var (
	//Sizes of communication structs
	ndbReqBytes  = int(C.sizeofNdbReq)
	ndbRespBytes = int(C.sizeofNdbResp)
	ndbHandleLen = int(C.ndbHandleLen)
	//Magic numbers
	nbdReqMagic   = uint32(C.htonl(C.uint32_t(C.NBD_REQUEST_MAGIC)))
	nbdReplyMagic = uint32(C.htonl(C.uint32_t(C.NBD_REPLY_MAGIC)))
	//NBD features
	ndbTrimSupported bool = C.ndbTrimSupported == 1
)

func newNbdRawReq() []byte {
	return make([]byte, ndbReqBytes)
}

func newNbdRawResp() []byte {
	return make([]byte, ndbReqBytes)
}

func newNbdHandle() []byte {
	return make([]byte, ndbHandleLen)
}

//DefaultBlockSize type meant to be embded in implemetations to easily provide
// the BlockSize() method
type DefaultBlockSize int64

//DefaultBlockSizeBytes is 4096 (4KB)
const DefaultBlockSizeBytes DefaultBlockSize = 4096

//BlockSize always returns DefaultBlockSizeBytes
func (DefaultBlockSize) BlockSize() int64 {
	return int64(DefaultBlockSizeBytes)
}
