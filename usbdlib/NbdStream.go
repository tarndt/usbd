package usbdlib

import (
	"log"
	"os"
	"sync"
	"syscall"

	"github.com/tarndt/usbd/errs"
)

//NbdStream manages the kernel-space resources that public a network block device
type NbdStream struct {
	socketFd int
	done     *sync.WaitGroup
}

func NewNbdStream(blockDeviceName string, dev Device) (*NbdStream, error) {
	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		return nil, errs.Append(err, "Could not create socket pair")
	}
	userSockFd, kernelSockFd := fds[0], uintptr(fds[1])
	//Open device file we will use to communicate to NDB (via ioctl)
	devFile, err := os.OpenFile(blockDeviceName, os.O_RDWR, 0)
	if err != nil {
		return nil, errs.Append(err, "Could not open NBD device: %s", blockDeviceName)
	}
	//Inform NBD of block size of our device
	if _, _, err = syscall.Syscall(syscall.SYS_IOCTL, devFile.Fd(), nbdSetBlockSize, uintptr(dev.BlockSize())); isNdbFail(err) {
		devFile.Close()
		return nil, errs.Append(err, "Could not inform NBD of device block size")
	}
	//Inform NBD of the size of our device (rounded to our block size)
	if _, _, err = syscall.Syscall(syscall.SYS_IOCTL, devFile.Fd(), nbdSetSizeBlocks, uintptr(dev.Size()/dev.BlockSize())); isNdbFail(err) {
		devFile.Close()
		return nil, errs.Append(err, "Could not inform NBD of the number of blocks on device")
	}
	//Reset the state of the socket with NBD
	if _, _, err = syscall.Syscall(syscall.SYS_IOCTL, devFile.Fd(), ndbClearSock, 0); isNdbFail(err) {
		devFile.Close()
		return nil, errs.Append(err, "Could not clear NBD socket")
	}
	//Inform NBD of the socket it should use to talk to us
	if _, _, err = syscall.Syscall(syscall.SYS_IOCTL, devFile.Fd(), nbdSetSock, kernelSockFd); isNdbFail(err) {
		devFile.Close()
		return nil, errs.Append(err, "Could not inform NBD of which socket to use")
	}
	//Ask NBD to send us trim commands, if supported
	if ndbTrimSupported {
		if _, _, err = syscall.Syscall(syscall.SYS_IOCTL, devFile.Fd(), nbdSetFlags, nbdFlagSendTrim); isNdbFail(err) {
			devFile.Close()
			return nil, errs.Append(err, "Could not inform NBD to send TRIM commands")
		}
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { //Finish setup...
		defer wg.Done()
		var err error
		//Ask NBD to begin service, this blocks until NBD service terminates
		if _, _, err = syscall.Syscall(syscall.SYS_IOCTL, devFile.Fd(), nbdDoIt, 0); isNdbFail(err) {
			log.Printf("NbdStream::NewNbdStream(): NBD Do it failed; Details: %s", err)
		}
		//Reset the state
		if err = syscall.Close(int(kernelSockFd)); isNdbFail(err) {
			log.Printf("NbdStream::NewNbdStream(): kernel side socket close failed; Details: %s", err)
		}
		if err = syscall.Close(userSockFd); err != nil {
			log.Printf("NbdStream::NewNbdStream(): user side socket close failed; Details: %s", err)
		}
		if _, _, err = syscall.Syscall(syscall.SYS_IOCTL, devFile.Fd(), ndbClearSock, 0); isNdbFail(err) {
			log.Printf("NbdStream::NewNbdStream(): NBD socket clear failed; Details: %s", err)
		}
		if _, _, err = syscall.Syscall(syscall.SYS_IOCTL, devFile.Fd(), nbdClearQueue, 0); isNdbFail(err) {
			log.Printf("NbdStream::NewNbdStream(): NBD queue clear failed; Details: %s", err)
		}
		if err = devFile.Close(); isNdbFail(err) {
			log.Printf("NbdStream::NewNbdStream(): NBD device file close failed; Details: %s", err)
		}

	}()
	go func() {
		//	We need to open the device again to ensure the OS rescans the partition table
		if tmp, err := os.OpenFile(blockDeviceName, os.O_RDONLY, 0); err != nil {
			log.Printf("NbdStream::NewNbdStream(): NBD device partition scan open failed; Details: %s", err)
		} else if err = tmp.Close(); err != nil {
			log.Printf("NbdStream::NewNbdStream(): NBD device partition scan close failed; Details: %s", err)
		}
	}()
	return &NbdStream{
		socketFd: userSockFd,
		done:     &wg,
	}, nil
}

func isNdbFail(err error) bool {
	if err != nil {
		if errno, ok := err.(syscall.Errno); !ok || errno != 0 {
			return true
		}
	}
	return false
}

func (this *NbdStream) Done() *sync.WaitGroup {
	return this.done
}

func (this *NbdStream) Read(buf []byte) (count int, err error) {
	return syscall.Read(this.socketFd, buf)
}

func (this *NbdStream) Write(buf []byte) (count int, err error) {
	return syscall.Write(this.socketFd, buf)
}
