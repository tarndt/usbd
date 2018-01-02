# usbd
User-Space Block Device (USBD) Framework (written in Go)

Many people are familiar with user-space file systems; this prototype is an attempt to similar provide user-space block devices, specifically those written in Go. USDB takes advantage of the seldom used [NBD](https://en.wikipedia.org/wiki/Network_block_device) interface provided by the Linux kernel to allow a daemon running in user-space to export a block device.

In this case a daemon is created using the USDB framework that acts as the NBD server. Overhead between the daemon and the kernel is minimized by using the [AF_UNIX](http://man7.org/linux/man-pages/man7/unix.7.html) (also known as AF_LOCAL) socket type to communicate between local processes efficiently. The USBD framework allows a Go programmer to define block devices that conform the the following simple interface:

```go
type Device interface {
	Size() int64
	BlockSize() int64
	ReadAt(buf []byte, pos int64) (count int, err error)
	WriteAt(buf []byte, pos int64) (count int, err error)
	Trim(pos int64, count int) error
	Flush() error
	Close() error
}
```

A `usbdlib.NbdStream` can then be configured to communicate with the kernel NDB facility and is passed to `usbdlib.ReqProcessor` which acts as a proxy between the kernel which is handling NDB requests and the users `usbdlib.Device` implementation.

```
├── devices
│   ├── filedisk
│   │   └── An example USDB implmentation that is backed by a simple file (filesystem)
│   ├── dedupdisk
│   │   ├── An example USDB implmentation that files to back a logical device,
|   |   |    but with the added use of hashing and a rocksDB database for block
|   |   |    level deduplication.
│   └── ramdisk
│       └── An example USBD implemenation that is memory backed (ramdrive)
├── errs
│   └── A barebones error handling library that extends Go's default error handling
├── usbdlib
│   ├── The core USB interface definitions and engine
└── usbdsrv
    ├── An example USBD server daemon
 ```
 ### Current state
 
 I have performed mostly manual testing at this point with the more extensive test being exporting an instance of the USBD deduplicating file-based block device. Then creating a VirtualBox VM using the exported NDB device and installing Windows XP or Ubuntu on it. I then (quick) re-formatted it, performed another fresh install and verified sw-duplication was taking place by checking the disk file-set did not grow meaningfully. I have seen read performance up to 1.4 GB/s and write performance (while writing duplicate data) as high as 1 GB/s with SSDs hosting the backing rocksDB database and block-file. 
 
 ### Building

This proof of concept is completely Linux centric and requires the kernel headers to be installed and available.

This project also uses https://github.com/alberts/gorocks for its RocksDB wrapper which is used in the sample implementation of the deduplicating file-based block device; this dependency requires setting the path to the rocksDB headers used by cGo:
 ```
 CGO_CFLAGS="-I/<path/to/rocksdb>/include"
 CGO_LDFLAGS="-L/<path/to/rocksdb>"
 ```
  
 ### Future
 
 If there is community interest in this proof of concept I am interested in extending this project, starting with:
1. Unit tests for usbdlib
2. Unit tests for the sample device implementations
3. Expect based integration tests for the sample usbd server daemon that test each of the example block devices
4. A new block device that extends the existing dedup one with compress as well as deduplication
