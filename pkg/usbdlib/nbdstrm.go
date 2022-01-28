package usbdlib

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"syscall"
)

//DefMaxNBDDevices if the NBD Linux kernel module is loaded and the user does
// not provide the number of NBD devices to allocate, this many are created.
// Important: The NBD kernel module does not support dynamic device creation after
// load via udev or mknod!
const DefMaxNBDDevices = 32

//NbdStream manages the kernel-space resources that are associated with a network block device (NBD)
type NbdStream struct {
	dev Device
	net.Conn
	ctx       context.Context
	ctxCancel context.CancelFunc
	errCh     chan error
}

//NewNbdHandler contructs a new NbdStream instance that handles requests for the
// provided Device.
func NewNbdHandler(ctx context.Context, dev Device, options ...interface{}) (*NbdStream, string, error) {
	var (
		blockDeviceName string
		err             error
	)

	maxNBDDevices := uint(DefMaxNBDDevices)
	if len(options) == 1 { //Args is a single arg telling us the max devices to create on nbd load
		var isInt bool
		if maxNBDDevices, isInt = options[0].(uint); isInt {
			options = nil
		}
	}

	if len(options) > 0 { //Args are one or more device names to try
		devNamesTried := make([]string, 0, len(options))
		var devName string
		for _, opt := range options {
			var isString bool
			devName, isString = opt.(string)

			if !isString {
				continue
			} else if err = useNbdDev(devName); err != nil {
				devNamesTried = append(devNamesTried, devName)
				continue
			}

			blockDeviceName = devName
			break
		}
		if blockDeviceName == "" {
			return nil, "", fmt.Errorf("Could not use any of provided device names: %s. Error on %q: %w", strings.Join(devNamesTried, ","), devName, err)
		}
	} else {
		blockDeviceName, err = makeNbdDev(maxNBDDevices)
		if err != nil {
			return nil, "", fmt.Errorf("Could not create NDB: %w", err)
		}
	}

	strm, err := newNbdStream(ctx, dev, blockDeviceName)
	return strm, blockDeviceName, err
}

func newNbdStream(ctx context.Context, dev Device, blockDeviceName string) (*NbdStream, error) {
	devFile, userSockFd, kernelSockFd, err := setupSycalls(blockDeviceName, dev)
	if err != nil {
		return nil, err
	}

	cmdSocket, err := net.FileConn(os.NewFile(uintptr(userSockFd), ""))
	if err != nil {
		return nil, fmt.Errorf("Could not create command socket")
	}

	//Error handling helpers
	errCh := make(chan error, 1)
	sendErr := func(err error) {
		select {
		case errCh <- err:
		default:
		}
	}

	ctx, cancel := context.WithCancel(ctx)

	var wgDone sync.WaitGroup
	wgDone.Add(2)
	nbdDoItWorker := func() { //Finish setup...
		var err error
		defer func() {
			wgDone.Done()
			cancel()
		}()

		//Ask NBD to begin service, this blocks until NBD service terminates
		_, _, err = sysCall(syscall.SYS_IOCTL, devFile.Fd(), nbdDoIt, 0)
		if err != nil {
			sendErr(fmt.Errorf("NBD \"Do it\" failed; Details: %w", err))
		}
	}

	var wgRunning sync.WaitGroup
	wgRunning.Add(1)
	scanPartTableThenCleanupWorker := func() {
		var err error
		defer func() {
			wgDone.Done()
			wgDone.Wait()

			//Cleanup
			if err = devFile.Close(); err != nil {
				sendErr(fmt.Errorf("NBD device file close failed; Details: %w", err))
			}
			if err = dev.Close(); err != nil && !errors.Is(err, context.Canceled) {
				sendErr(fmt.Errorf("NBD user space device close failed; Details: %w", err))
			}
			if err = syscall.Close(kernelSockFd); err != nil {
				sendErr(fmt.Errorf("kernel side socket close failed; Details: %w", err))
			}
			if err = syscall.Close(userSockFd); err != nil && !strings.Contains(err.Error(), "bad file descriptor") {
				//This failing with bad desc is normal if ndbClearSock did its job
				sendErr(fmt.Errorf("user side socket close failed; Details: %w", err))
			}
			close(errCh) //unblock Close()
		}()

		//We need to open the device again to ensure the OS rescans the partition table
		if tmp, err := os.OpenFile(blockDeviceName, os.O_RDONLY, 0); err != nil {
			sendErr(fmt.Errorf("NBD %q partition scan open failed; Details: %w", blockDeviceName, err))
			wgRunning.Done()
			return
		} else if err = tmp.Close(); err != nil {
			sendErr(fmt.Errorf("NBD %q partition scan close failed; Details: %w", blockDeviceName, err))
			wgRunning.Done()
			return
		}

		wgRunning.Done()
		<-ctx.Done()
		//Disconnect and reset NBD driver state
		if _, _, err = sysCall(syscall.SYS_IOCTL, devFile.Fd(), nbdClearQueue, 0); err != nil {
			sendErr(fmt.Errorf("NBD queue clear failed; Details: %w", err))
		}
		if _, _, err = sysCall(syscall.SYS_IOCTL, devFile.Fd(), ndbDisconnect, 0); err != nil {
			sendErr(fmt.Errorf("NBD disconnect failed; Details: %w", err))
		}
		if _, _, err = sysCall(syscall.SYS_IOCTL, devFile.Fd(), ndbClearSock, 0); err != nil {
			sendErr(fmt.Errorf("NBD socket clear failed; Details: %w", err))
		}
	}

	//Kick things off
	go nbdDoItWorker()
	go scanPartTableThenCleanupWorker()
	wgRunning.Wait()

	//Check for errors so far
	select {
	case err = <-errCh:
		if err != nil {
			cancel() //Abort!
			return nil, err
		}
	default:
	}

	return &NbdStream{
		dev:       dev,
		Conn:      cmdSocket,
		ctx:       ctx,
		ctxCancel: cancel,
		errCh:     errCh,
	}, nil
}

//Close this NbdStream
func (strm *NbdStream) Close() error {
	strm.ctxCancel()
	return <-strm.errCh
}

//ProcessRequests for this NbdStream. Blocks until Close is called, so you may
// want to run this in secondary goroutine.
func (strm *NbdStream) ProcessRequests() error {
	return processRequests(context.Background(), strm, strm.dev, RecommendWorkerCount())
}

func setupSycalls(blockDeviceName string, dev Device) (*os.File, int, int, error) {
	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("Could not create socket pair: %w", err)
	}
	userSockFd, kernelSockFd := fds[0], uintptr(fds[1])

	//Open device file we will use to communicate to NDB (via ioctl)
	devFile, err := os.OpenFile(blockDeviceName, os.O_RDWR, 0)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("Could not open NBD device file: %s: %w", blockDeviceName, err)
	}

	//Inform NBD of block size of our device
	if _, _, err = sysCall(syscall.SYS_IOCTL, devFile.Fd(), nbdSetBlockSize, uintptr(dev.BlockSize())); err != nil {
		devFile.Close()
		return nil, 0, 0, fmt.Errorf("Could not inform NBD of device block size: %w", err)
	}

	//Inform NBD of the size of our device (rounded to our block size)
	if _, _, err = sysCall(syscall.SYS_IOCTL, devFile.Fd(), nbdSetSizeBlocks, uintptr(dev.Size()/dev.BlockSize())); err != nil {
		devFile.Close()
		return nil, 0, 0, fmt.Errorf("Could not inform NBD of the number of blocks on device: %w", err)
	}

	//Reset the state of the socket with NBD
	if _, _, err = sysCall(syscall.SYS_IOCTL, devFile.Fd(), ndbClearSock, 0); err != nil {
		devFile.Close()
		return nil, 0, 0, fmt.Errorf("Could not clear NBD socket: %w", err)
	}

	//Inform NBD of the socket it should use to talk to us
	if _, _, err = sysCall(syscall.SYS_IOCTL, devFile.Fd(), nbdSetSock, kernelSockFd); err != nil {
		devFile.Close()
		return nil, 0, 0, fmt.Errorf("Could not inform NBD of which socket to use: %w", err)
	}

	//Ask NBD to send us trim commands, if supported
	if ndbTrimSupported {
		if _, _, err = sysCall(syscall.SYS_IOCTL, devFile.Fd(), nbdSetFlags, nbdFlagSendTrim); err != nil {
			devFile.Close()
			return nil, 0, 0, fmt.Errorf("Could not inform NBD to send TRIM commands: %w", err)
		}
	}

	return devFile, userSockFd, int(kernelSockFd), nil
}

func sysCall(trap, a1, a2, a3 uintptr) (r1, r2 uintptr, err error) {
	var errNo syscall.Errno
	r1, r2, errNo = syscall.Syscall(trap, a1, a2, a3)
	if errNo != 0 {
		err = errNo
	}
	return
}
