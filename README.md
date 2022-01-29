# usbd
User-Space Block Device (USBD) Framework (written in Go)
 [![License: MPL 2.0](https://img.shields.io/badge/License-MPL_2.0-brightgreen.svg)](https://opensource.org/licenses/MPL-2.0)[![Go Reference](https://pkg.go.dev/badge/github.com/tarndt/usbd.svg)](https://pkg.go.dev/github.com/tarndt/usbd) [![Go Report Card](https://goreportcard.com/badge/github.com/tarndt/usbd)](https://goreportcard.com/report/github.com/tarndt/usbd)

Many people are familiar with user-space file systems; this library is an attempt to similar provide user-space block devices, specifically those written in Go. USDB takes advantage of the seldom used [NBD](https://en.wikipedia.org/wiki/Network_block_device) interface provided by the Linux kernel to allow a daemon running in user-space to export a block device. This package can be used to write software block devices and export them via /dev/nbdX. After doing so such a device can be formatted with the filesystem of the users'choice (ex. ext4, btrfs, xfs, etc) and mounted in the usual ways (ex. mount, /etc/fstab, etc).

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
cmd
├── usbdsrvd
│   │   └── An example deamon that use usbdlib to serve this repositories user-space
│   │       device implemenations.
│   └── conf
│       └── usbdsrvd configuration definitions and logic.
pkg
├── devices
│   ├── filedisk
│   │   └── An example USDB implmentation that is backed by a simple file (filesystem).
│   ├── dedupdisk
│   │   └── An example, but potentially useful, USDB implmentation that uses files to
│   │       back a logical device, but with the added use of hashing and a Pebble database
│   │       for block level deduplication. This is an older prototype and file/ramdisk are
│   │       better examples for those looking to implement their own devices.
│   ├── ramdisk
│   │   └── An example USBD implemenation that is memory backed (ramdrive).
│   └── objstore
│       └── A USDB implmentation, which is practically useful as it creates a logical
│           device which is cached locally by actually backed by a remote objectstore
│           of the users choice. (Ex. S3/minio, Swift, BackBlaze, Azure/Google Object Storage).
│           However, due to its complexity is not a great example for learning how to build
│           a user-space block device.
├── usbdlib
│   └── The core USB interface definitions and engine
└── util
    └── Utilities for implementors of user-space block devices to reuse    
```
Note the objstore and dedupdisk implemenations are non-trival implemenation compared to the others and use the [stow ObjectStorage abstraction library](https://github.com/graymeta/stow) and [Pebble DB](https://github.com/cockroachdb/pebble) respectively.
 
### Current state

#### General
 
In addition to basic unit tests:

```
$ go test ./...
ok  	github.com/tarndt/usbd/pkg/devices/dedupdisk/dedupdisk_test	6.166s
ok  	github.com/tarndt/usbd/pkg/devices/filedisk/filedisk_test	8.011s
ok  	github.com/tarndt/usbd/pkg/devices/objstore	65.617s
ok  	github.com/tarndt/usbd/pkg/devices/ramdisk	2.351s
ok  	github.com/tarndt/usbd/pkg/usbdlib	(cached)
ok  	github.com/tarndt/usbd/pkg/usbdlib/usbdlib_test	0.197s
ok  	github.com/tarndt/usbd/pkg/util	(cached)
```

Manual testing has been performed using standard block device tools and by exporting an instance of the USBD reference implemenations and creating a VirtualBox VM using the exported NDB device and installing Windows XP, Windows 10 and Ubuntu on it. 

#### ObjectStore

This OS install procedure has been used to verify correct operation of the objectstore implemenation. Key to this implemenation preforming well use of the included [S2 compression](https://github.com/klauspost/compress/tree/master/s2) and fast (enough) network access to the backing objectstore server. For security reasons its highly recommended to use the provided AES encryption functionality.

The ObjectStore implemenation uses ([a fork](https://github.com/tarndt/stow) due to fixes) of the [stow ObjectStorage abstraction library](https://github.com/graymeta/stow) to support many popular object stores. The command line argument `-objstore-cfg=<yourJSON>` allows for ObjectStore specific configuration parameters to be passed, so see what these parameters look like, take a look at the per ObjectStore configuration. For examplem see [S3 config](https://github.com/tarndt/stow/blob/master/s3/config.go#L24-L53) or [Azure config](https://github.com/tarndt/stow/blob/master/azure/config.go#L13-L16) options.

An easy way to test this is to setup a local [minio](https://min.io/) (S3 compatible) objectstore server:

```
cd /tmp
wget https://dl.min.io/server/minio/release/linux-amd64/minio
chmod u+x ./minio
mkdir /minio_data
./minio server ./minio_data
```

The minio server instance will use the default minio credentials which are in turn the defaults used by usbdsrvd when objectstore config is not provided.
 
#### Deduplication

To verify the dedup implemenation, after initial installaion testing the disk was then "quick" (no zeroing) re-formatted, and another fresh install was performed and file analysis verified duplication was taking place by checking the disk files did not grow meaningfully. Read performance up to 3.2 GB/s and write performance (while writing duplicate data) as high as 2.1 GB/s with SSDs hosting the backing PebbleDB database and block-file as been achieved. 

### Building

This implemenation is completely Linux centric (since it uses NBD) and requires the kernel headers to be installed and available.

### Running

While this library is intended to be used by other daemons, the included usbdsrvd (main.go) will host instances of the sample device implemenations and may be useful in its own right.

```
Usage: ./usbdsrvd [optional: options see below...] [optional: NBD device to use ex. /dev/nbd0, if absent the first free device is used.]
Arguments starting with <driver name>-X are only applicable if dev-type=X is being set.
	Example:
		1 GiB device backed with by memory: ./usbdsrvd
		8 GiB device backed by a file and exported specfically on /dev/nbd5: ./usbdsrvd -dev-type=file -store-dir=/tmp -store-name=testfilevol -store-size=8GiB /dev/nbd5
		12 GiB device backed by file deduplicated using PebbleDB: ./usbdsrvd -dev-type=file -store-dir=/tmp -store-name=testdedupvol -store-size=12GiB
		20 GiB device backed by a locally running S3/minio objectstore: ./usbdsrvd -dev-type=objstore -store-dir=/tmp -store-name=testobjvol -store-size=20GiB

  -help
    	Display help and exit
  -dev-type string
    	Type of device to back block device with: 'mem', 'file', 'dedup', 'objstore'. (default "mem")
  -store-dir string
    	Location to create new backing disk files in (default "./")
  -store-name string
    	File base name to use for new backing disk files (default "test-lun")
  -store-size value
    	Amount of storage capcity to use for new backing files (ex. 100 MiB, 20 GiB) (default 1.0 GiB)
  -nbd-max-devs uint
    	If the NBD kernel module is loaded by this deamon how many NBD devices should it create (default 32)

  -dedup-memcache string
    	Amount of memory to the dedup store ID cache (ex. 100 MiB, 20 GiB) (default "512 MiB")

  -objstore-kind string
    	Type of remote objectstore: 's3', 'b2', 'local', 'azure', 'swift', 'google', 'oracle' or 'sftp' (default "s3")
  -objstore-cfg string
    	JSON configuration (default assumes local minio [kind "s3"] with default settings) (default "{\"access_key_id\":\"minioadmin\",\"endpoint\":\"http://127.0.0.1:9000\",\"secret_key\":\"minioadmin\"}")
  -objstore-objsize value
    	Size of remote objects (ex. 32 MiB, 1 GiB) (default 64 MiB)
  -objstore-diskcache value
    	Amount of disk for caching remote objects (0 implies fullbacking or ex. 100 MiB, 20 GiB)

  -objstore-aeskey string
    	If AES is enabled; AES key to use to encrypt remote objects (if absent a key is generated and saved to ./key.aes, otherwise use: key:<value>, file:<path>, env:<varname>
  -objstore-aesmode string
    	AES encryption mode to use to encrypt remote objects: "aes-cfb", "aes-ctr", "aes-ofb" or "identity" for no encryption. "aes-ctr" is recommended. (default "aes-ctr")

  -objstore-compress string
    	Compression algorithm to use for remote objects: "s2", "gzip" or "identity" for no compression) (default "s2")

  -objstore-concurflush uint
    	Maximum number of dirty local objects to concurrently upload to the remote objectstore (0 implies use heuristic)
  -objstore-flushevery duration
    	Frequency in which dirty local objects are uploaded to the remote objectstore (0 disables autoflush) (default 10s)
```

### Future
 
As of writing, this package is actively maintained. Issue reports are welcome. Pull requests are welcome but will require consent to a contributor agreement.

Work is in progress for a [CSI storage driver](https://kubernetes-csi.github.io/docs/drivers.html) that uses the objectstore implemenation provided in this repository. If you are interested in contributing please contact the author!
