
# usbd

A User-Space Block Device (USBD) Framework (written in Go)
 [![License: MPL 2.0](https://img.shields.io/badge/License-MPL_2.0-brightgreen.svg)](https://opensource.org/licenses/MPL-2.0)[![Go Reference](https://pkg.go.dev/badge/github.com/tarndt/usbd.svg)](https://pkg.go.dev/github.com/tarndt/usbd) [![Go Report Card](https://goreportcard.com/badge/github.com/tarndt/usbd)](https://goreportcard.com/report/github.com/tarndt/usbd)

## Introduction

Many people are familiar with [user-space](https://en.wikipedia.org/wiki/User_space) [filesystem](https://en.wikipedia.org/wiki/File_system); on [Linux](https://en.wikipedia.org/wiki/Linux) these are popularly implemented using [FUSE](https://en.wikipedia.org/wiki/Filesystem_in_Userspace). This library is an attempt to similar provide similar facility for user-space [block devices](https://en.wikipedia.org/wiki/Device_file#BLOCKDEV), specifically those written in [Go](https://en.wikipedia.org/wiki/Go_(programming_language)). USBD takes advantage of the seldom used [NBD](https://en.wikipedia.org/wiki/Network_block_device) interface provided by an in tree Linux kernel module to allow a daemon running in user-space to export a block device. This package can be used to write software block device (in Go) and export it via `/dev/nbdX` where *X* is the next available device on the system. After doing so, such a device can be formatted with the [filesystem](https://en.wikipedia.org/wiki/File_system) of the user's choice (ex. [ext4](https://en.wikipedia.org/wiki/Ext4), [btrfs](https://en.wikipedia.org/wiki/Btrfs), [xfs](https://en.wikipedia.org/wiki/XFS), [zfs](https://en.wikipedia.org/wiki/ZFS), etc) and mounted in the usual ways (ex. [mount](https://man7.org/linux/man-pages/man8/mount.8.html), [/etc/fstab](https://en.wikipedia.org/wiki/Fstab), etc).

A [daemon](https://en.wikipedia.org/wiki/Daemon_(computing)) is created using the USBD framework that acts as an NBD server. Overhead between the user-space daemon and the [kernel](https://en.wikipedia.org/wiki/Kernel_(operating_system)) is minimized by using the [AF_UNIX](http://man7.org/linux/man-pages/man7/unix.7.html) (also known as AF_LOCAL) socket type to communicate between local processes efficiently. The USBD framework allows a Go programmer to define their own block devices by implementing a [type](https://go.dev/ref/spec#Struct_types) that conforms the following [simple interface](https://github.com/tarndt/usbd/blob/master/pkg/usbdlib/dev.go):

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

Typically a daemon will call [NewNbdHandler](https://github.com/tarndt/usbd/blob/master/pkg/usbdlib/nbdstrm.go#L31) to instantiate a new [usbdlib.NbdStream](https://github.com/tarndt/usbd/blob/master/pkg/usbdlib/nbdstrm.go#L21) which is configured to communicate with the kernel NDB module and is passed to [usbdlib.ReqProcessor](https://github.com/tarndt/usbd/blob/master/pkg/usbdlib/reqproc.go#L22) by calling [NbdStream.ProcessRequests](https://github.com/tarndt/usbd/blob/c04abfb943dd2070e2dbc541632ca8c069ed6b8a/pkg/usbdlib/nbdstrm.go#L196) which acts as a proxy between the kernel which is handling NDB requests and the users [usbdlib.Device](ttps://github.com/tarndt/usbd/blob/master/pkg/usbdlib/dev.go) implementation. A command-line server, [usbdsrvd](https://github.com/tarndt/usbd/tree/master/cmd/usbdsrvd), is included to allow easy testing of the USBD engine and reference devices. In its implementation the above initialization procedure can be [observed](https://github.com/tarndt/usbd/blob/master/cmd/usbdsrvd/main.go#L36-L58). This server will make a [chosen device](https://github.com/tarndt/usbd/tree/master/pkg/devices) available as `/dev/nbdX` after which time it can be formatted and mounted or otherwise used [as any other block device](https://www.digitalocean.com/community/tutorials/how-to-partition-and-format-storage-devices-in-linux).

### Project Structure

```
cmd
├── usbdsrvd
│   │   └── An example daemon that use usbdlib to serve this repositories user-space
│   │       device implementations.
│   └── conf
│       └── usbdsrvd configuration definitions and logic.
pkg
├── devices
│   ├── filedisk
│   │   └── An example USBD implementation that is backed by a simple file (filesystem).
│   ├── dedupdisk
│   │   └── An example, but potentially useful, USBD implementation that uses files to
│   │       back a logical device, but with the added use of hashing and a Pebble database
│   │       for block level deduplication. This is an older prototype and file/ramdisk are
│   │       better examples for those looking to implement their own devices.
│   ├── ramdisk
│   │   └── An example USBD implementation that is memory backed (ramdrive).
│   └── objstore
│       └── A USBD implementation, which is practically useful as it creates a logical
│           device which is cached locally but actually backed by a remote objectstore
│           of the users choice. (Ex. S3/minio, Swift, BackBlaze, Azure/Google Object Storage).
│           However, due to its complexity is not a great example for learning how to build
│           a user-space block device.
├── usbdlib
│   └── The core USBD interface definitions and engine to export/serve implementations
└── util
    └── Utilities for implementors of user-space block devices to reuse    
```
*Note the [objstore](https://github.com/tarndt/usbd/tree/master/pkg/devices/objstore) and [dedupdisk](https://github.com/tarndt/usbd/tree/master/pkg/devices/dedupdisk) implementations are non-trivial implementation compared to the others and use the [stow ObjectStorage abstraction library](https://github.com/graymeta/stow) and [Pebble](https://github.com/cockroachdb/pebble) DB respectively.*
 
## Current state

As of writing, this package is actively maintained. Issue reports are welcome. Pull requests are welcome but will require consent to a contributor agreement. This project is currently beta quality. There are no known defects or risks to usage, but it has not yet been widely used. Experience reports are helpful and encouraged!

### Basic Reference Devices (ramdisk and filedisk)

There are [ramdisk](https://github.com/tarndt/usbd/tree/master/pkg/devices/ramdisk) and [filedisk](https://github.com/tarndt/usbd/tree/master/pkg/devices/filedisk) device implementations that are excellent for understanding how to implement a simple [USBD device]((https://github.com/tarndt/usbd/blob/master/pkg/usbdlib/dev.go)). Starting [usbdsrvd](https://github.com/tarndt/usbd/tree/master/cmd/usbdsrvd) with defaults (no arguments) will result in a 1 GB [ramdisk](https://github.com/tarndt/usbd/tree/master/pkg/devices/ramdisk) backed device being exposed as the next available NBD device, typically `/dev/nbd0`. Unlike these straight forward examples, there are also some [advanced device implemenations](https://github.com/tarndt/usbd#advanced-devices) which may be of more practical value.

### Building

This project is completely Linux centric (since it uses NBD) and requires [the NBD kernel headers](https://github.com/torvalds/linux/blob/5bfc75d92efd494db37f5c4c173d3639d4772966/include/uapi/linux/nbd.h) to be installed and available. To build the [Go toolchain](https://pkg.go.dev/cmd/go) must be [installed](https://go.dev/doc/install) after that building is a simple matter of running `go build` in [the usbdsrvd directory](https://github.com/tarndt/usbd/tree/master/cmd/usbdsrvd) to generate an executable.

### Testing

usbd has both automated unit testing and manual testing approaches.

#### Unit Testing

There is a provided [a suite of basic unit tests for implementors](https://github.com/tarndt/usbd/blob/master/pkg/devices/testutil/suites.go) of devices that all of the included devices use:
```
$ cd usbd
$ go test ./...
ok  	github.com/tarndt/usbd/pkg/devices/dedupdisk/dedupdisk_test	6.166s
ok  	github.com/tarndt/usbd/pkg/devices/filedisk/filedisk_test	8.011s
ok  	github.com/tarndt/usbd/pkg/devices/objstore	65.617s
ok  	github.com/tarndt/usbd/pkg/devices/ramdisk	2.351s
ok  	github.com/tarndt/usbd/pkg/usbdlib	(cached)
ok  	github.com/tarndt/usbd/pkg/usbdlib/usbdlib_test	0.197s
ok  	github.com/tarndt/usbd/pkg/util	(cached)
```
Executing these tests is just a matter of running `go test`. As seen above you can run all unit tests in the project repository by running `go test ./...` in the root directory. Some [tests that interact with the Linux kernel](https://github.com/tarndt/usbd/blob/master/pkg/devices/testutil/suites.go#L41) require [super-user privileges](https://en.wikipedia.org/wiki/Superuser) (aka `root`). Should you run tests with verbosity (`go test -v`), you will see some tests are skipped during normal execution. Additionally, while running tests with the race detector (`-race`) has proven fruitful for discovering data races, please be warned that in this mode tests often take an order of magnitude longer to run and may require you to increase the test timeout (ex. `-timeout=10m`). The provided test suits do support a short mode as well (`go test -short`). Sometimes its useful to compile a package's unit tests to its own executable, and this can be done in typical Go fashion `go test -c` (or `go test -c -race` to also enable the data race detector).

#### Manual Testing

Manual testing has been performed using standard block device tools (ex. [hdparm](https://en.wikipedia.org/wiki/Hdparm)) and by exporting an instance of the USBD reference implementations and creating a [VirtualBox](https://en.wikipedia.org/wiki/VirtualBox) [VM](https://en.wikipedia.org/wiki/Virtual_machine) using the exported NDB device and installing [Windows](https://en.wikipedia.org/wiki/Microsoft_Windows) XP/10 and [Ubuntu](https://ubuntu.com/) on it. 

### Advanced Devices

In addition to the basic reference devices, as mentioned in the project structure above, there are two notable non-trivial device implemenations.

#### ObjectStore (objstore)

The ObjectStore ([objstore](https://github.com/tarndt/usbd/tree/master/pkg/devices/objstore)) driver creates a logical device which is cached locally on disk but actually backed by a remote [object storage](https://en.wikipedia.org/wiki/Object_storage) of the users choice (ex. [S3](https://en.wikipedia.org/wiki/Amazon_S3), [Swift](https://wiki.openstack.org/wiki/Swift), BackBlaze [B2](https://en.wikipedia.org/wiki/Backblaze#Backblaze_B2_Storage) and [Azure's](https://azure.microsoft.com/en-us/services/storage/blobs/)/[Google's](https://cloud.google.com/storage) Object Storage). Besides the original implementors and providers of each object storage technology/service there are other projects and organizations that provide compatible alternatives for them; for example, [minio](https://min.io/) allows easy self-hosted/[Kubernetes](https://en.wikipedia.org/wiki/Kubernetes)-hosted S3, [Ceph](https://docs.ceph.com/en/pacific/) emulates both S3 and Swift with is [rados gateway](https://docs.ceph.com/en/pacific/radosgw/) and many other cloud providers offer S3-compatible SaaS such as [DigitalOcean's Spaces](https://www.digitalocean.com/products/spaces/) and [wasabi](https://wasabi.com/rcs/). 

The above OS install procedure has been used to manually verify correct operation of the [objectstore](https://github.com/tarndt/usbd/tree/master/pkg/devices/objstore) implementation. Key to this implementation preforming well is the use of the included [S2 compression](https://github.com/klauspost/compress/tree/master/s2) and fast (enough) network access to the [backing objectstore server](https://github.com/tarndt/usbd/blob/master/pkg/devices/objstore/stowutil.go#L22-L29). For security reasons its highly recommended to use [the provided AES encryption](https://github.com/tarndt/usbd/blob/master/pkg/devices/objstore/options.go#L53-L57) functionality.

Due to fixes that have not yet made it upstream the [ObjectStore implementation](https://github.com/tarndt/usbd/tree/master/pkg/devices/objstore) uses [a fork](https://github.com/tarndt/stow) of the [stow](https://github.com/graymeta/stow) ObjectStorage abstraction library which enables this project to easily support [many popular object stores](https://github.com/tarndt/usbd/blob/master/pkg/devices/objstore/stowutil.go#L22-L29). The command line argument `-objstore-cfg=<yourJSON>` allows for ObjectStore type specific configuration parameters to be passed. To see examples these parameters refer to the stow ObjectStore configuration definitions, for example, [S3 config](https://github.com/tarndt/stow/blob/master/s3/config.go#L24-L53) or [Azure config](https://github.com/tarndt/stow/blob/master/azure/config.go#L13-L16) options.

An easy way to test this is to setup a local [minio](https://min.io/) (S3 compatible) objectstore server:

```
cd /tmp
wget https://dl.min.io/server/minio/release/linux-amd64/minio
chmod u+x ./minio
mkdir /minio_data
./minio server ./minio_data
```

The minio server instance will use the default minio credentials which are in turn the defaults used by `usbdsrvd` when objectstore config is not provided.
 
#### Deduplication (dedupdisk)

The Deduplication ([dedupdisk](https://github.com/tarndt/usbd/tree/master/pkg/devices/dedupdisk)) device implementation uses files to back a logical device, but with the added use of [content-hashing](https://en.wikipedia.org/wiki/Hash_function) and a [Pebble](https://github.com/cockroachdb/pebble) database for [block-level deduplication](https://en.wikipedia.org/wiki/Data_deduplication). Work is [in progress](https://github.com/tarndt/usbd/blob/master/pkg/devices/dedupdisk/impls/lz4blkstore.go#L49) to allow the blocks that must be stored to also be compressed.

To verify the [dedup implementation](https://github.com/tarndt/usbd/tree/master/pkg/devices/dedupdisk), after initial installation testing the disk was then "quick" (no zeroing) re-formatted, and another fresh install was performed and file analysis verified duplication was taking place by checking the disk files did not grow meaningfully. Read performance up to 3.2 GB/s and write performance (while writing duplicate data) as high as 2.1 GB/s with SSDs hosting the backing PebbleDB database and block-file as been achieved. 

### Running

While this library is intended to be used by other daemons, the included [usbdsrvd](https://github.com/tarndt/usbd/tree/master/cmd/usbdsrvd) ([main.go](https://github.com/tarndt/usbd/blob/master/cmd/usbdsrvd/main.go)) will host instances of the [sample device implementations](https://github.com/tarndt/usbd/tree/master/pkg/devices) and may be useful in its own right. Starting [usbdsrvd](https://github.com/tarndt/usbd/tree/master/cmd/usbdsrvd) with defaults (no arguments) will result in a 1 GB [ramdisk](https://github.com/tarndt/usbd/tree/master/pkg/devices/ramdisk) backed device being exposed as the next available NBD device typically `/dev/nbd0`. If the NBD kernel module  is not loaded `usbdsrvd` [will attempt to load it](https://github.com/tarndt/usbd/blob/master/pkg/usbdlib/nbdkern_linux.go#L84-L109). The maximum number of NBD devices a system can have [is determined at kernel module load time](https://github.com/torvalds/linux/blob/master/drivers/block/nbd.c#L2510-L2512) so if the [default](https://github.com/tarndt/usbd/blob/master/pkg/usbdlib/nbdstrm.go#L18) is too few devices you may need to increase it with `-nbd-max-devs` if using the `usbdsrvd` daemon or via [an argument](https://github.com/tarndt/usbd/blob/master/cmd/usbdsrvd/main.go#L41) to [NewNbdHandler](https://github.com/tarndt/usbd/blob/c04abfb943dd2070e2dbc541632ca8c069ed6b8a/pkg/usbdlib/nbdstrm.go#L31) if interfacing programmatically.

```
Usage: ./usbdsrvd [optional: options see below...] [optional: NBD device to use ex. /dev/nbd0, if absent the first free device is used.]
Arguments starting with <driver name>-X are only applicable if dev-type=X is being set.
	Example:
		1 GiB device backed with by memory: ./usbdsrvd
		8 GiB device backed by a file and exported specifically on /dev/nbd5: ./usbdsrvd -dev-type=file -store-dir=/tmp -store-name=testfilevol -store-size=8GiB /dev/nbd5
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
    	If the NBD kernel module is loaded by this daemon how many NBD devices should it create (default 32)

  -dedup-memcache string
    	Amount of memory to the dedup store ID cache (ex. 100 MiB, 20 GiB) (default "512 MiB")

  -objstore-kind string
    	Type of remote objectstore: 's3', 'b2', 'local', 'azure', 'swift', 'google', 'oracle' or 'sftp' (default "s3")
  -objstore-cfg string
    	JSON configuration (default assumes local minio [kind "s3"] with default settings) (default "{\"access_key_id\":\"minioadmin\",\"endpoint\":\"http://127.0.0.1:9000\",\"secret_key\":\"minioadmin\"}")
  -objstore-objsize value
    	Size of remote objects (ex. 32 MiB, 1 GiB) (default 64 MiB)
  -objstore-diskcache value
    	Amount of disk for caching remote objects (0 implies local fullbacking/caching or ex. 100 MiB, 20 GiB)
  -objstore-persistcache
    	Should the local cache be persistent or deleted on device shutdown

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

### Usage Tutorial: objstore

Lets create a 10 GB volume using local minio (S3 compatible) server. Please note that many of the operations below require [super-user privileges](https://en.wikipedia.org/wiki/Superuser) (aka `root`).

***Terminal Session 1**: Setup minio server*
```
cd /tmp
wget https://dl.min.io/server/minio/release/linux-amd64/minio
chmod u+x ./minio
mkdir ./minio_data
./minio server ./minio_data
```
Note the data in `/tmp/minio_data` would normally live on a remote service.

You should see minio is running:
```
WARNING: Detected default credentials 'minioadmin:minioadmin', we recommend that you change these values with 'MINIO_ROOT_USER' and 'MINIO_ROOT_PASSWORD' environment variables
......
WARNING: Console endpoint is listening on a dynamic port (44599), please use --console-address ":PORT" to choose a static port.
```

***Terminal Session 2**: Start a usbd server*
```
cd /tmp
mkdir ./usbdsrv_cache
git clone https://github.com/tarndt/usbd.git
cd usbd/cmd/usbdsrvd
go build
sudo ./usbdsrvd -dev-type=objstore -store-dir=/tmp/usbdsrv_cache -store-name=tutorial-vol -store-size=10GiB
```

You should see the block device is being exported:
```
Generated AES-256 key and stored it in "/tmp/usbdsrv_cache/key.aes"
USBD Server (./usbdsrvd) started.
USBD Server (./usbdsrvd) using config: Exporting 20 GiB volume "tutorial-vol" as next available NBD device with local storage at "/tmp/usbdsrv_cache" using driver locally-cached objectstore using a s3 remote object store (secret_key=<REDACTED>, access_key_id="minioadmin", endpoint="http://127.0.0.1:9000"), with 64 MiB objects, using up to as much as total device size of persistent local storage for cache, s2 compression, aes-ctr encryption, flushing to remote story every 10s using 16 workers
USBD Server (./usbdsrvd) is processing requests for "/dev/nbd0"
```
Take note the device the volume is being exported on, the following instructions assume "/dev/nbd0" as shown above. Later, you can shutdown this service using ``Ctrl+C`` or `kill` (but make sure you umount the device first!).

***Terminal Session 3**: Using it!*
You can now format the device and start using it:
```
sudo mkfs.ext4 /dev/nbd0
cd /tmp
mkdir ./tutorial-vol
sudo mount /dev/nbd0 /tmp/tutorial-vol
sudo chmod a+r,a+w /tmp/tutorial-vol/
```
*The device should show as mounted and files written to `/tmp/tutorial-vol` are now backed with the (in this case the local minio) objectstore.*

```
$ mount | grep nbd
/dev/nbd0 on /tmp/tutorial-vol type ext4 (rw,relatime)
$ cd /tmp/tutorial-vol/
$ echo "test file" > test.txt
$ cat test.txt
test file
```
*Feel free to inspect `usbdsrvd`'s cache and/or `minio`'s data directories to observe the presence of data files.*

To cleanup ensure you are not in ``/tmp/tutorial-vol`` (or you will get a busy error) and umount.
```
cd /tmp
sudo umount /tmp/tutorial-vol
```

You can now return to session 2, ``Ctrl+C`` usbdsrvd, and then finally session 3 and ``Ctrl+C`` minio. To avoid data loss, it is key that correct shutdown order is followed:
1. Cease usage of mount point
2. unmount block device
3. daemon shutdown
4. objectstore shutdown

## Future

Work is in progress for an patch to the Linux kernel NBD driver to allow dynamic creation of NBD devices in addition to the pre-allocation at module load provided for today. This will allow facilities such as [udev](https://en.wikipedia.org/wiki/Udev) and the `mknod` [system call](https://man7.org/linux/man-pages/man2/mknod.2.html[) or [command](https://man7.org/linux/man-pages/man1/mknod.1.html) to provision more NBD devices then intitally created.

Work is in progress for a [CSI storage driver](https://kubernetes-csi.github.io/docs/drivers.html) that uses the objectstore implementation provided in this repository. This will allow [Kubernetes](https://en.wikipedia.org/wiki/Kubernetes) to use the provideded object-storage backed device to provide [persistant volumes](https://kubernetes.io/docs/concepts/storage/persistent-volumes/). If you are interested in contributing please contact the author!
