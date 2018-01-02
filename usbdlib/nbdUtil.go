package usbdlib

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
	//NBD commands
	nbdRead    = uint32(C.NBD_CMD_READ)
	nbdWrite   = uint32(C.NBD_CMD_WRITE)
	nbdDiscard = uint32(C.NBD_CMD_DISC)
	nbdTrim    = uint32(C.NBD_CMD_TRIM)
)

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

type DefaultBlockSize uint64

func (that DefaultBlockSize) BlockSize() int64 {
	return 4096
}
