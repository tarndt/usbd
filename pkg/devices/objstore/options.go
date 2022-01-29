package objstore

import (
	"time"

	"github.com/tarndt/usbd/pkg/devices/objstore/compress"
	"github.com/tarndt/usbd/pkg/devices/objstore/encrypt"
)

//Option is an ObjectStore device option
type Option interface {
	apply(*device)
}

//OptAutoflushIval instructs an ObjectStore device to flush every interval
type OptAutoflushIval time.Duration

func (interval OptAutoflushIval) apply(dev *device) {
	dev.autoflush = time.Duration(interval)
}

//OptConcurFlushCount instructs an ObjectStore device to use this many concurrent workers
// this has memory usage implications (workers * objectsize)
type OptConcurFlushCount uint

func (count OptConcurFlushCount) apply(dev *device) {
	dev.concurFlush = uint(count)
}

//OptCompressRemoteObjects instructs an ObjectStore device to use the provided compression mode
type OptCompressRemoteObjects compress.Mode

func (compressMode OptCompressRemoteObjects) apply(dev *device) {
	dev.compressMode = compress.Mode(compressMode)
}

//OptNoMetadataSupport instructs an ObjectStore device reject configurations that
// require the provided stow.Location to support metadata. (ex. Compression)
type OptNoMetadataSupport bool

func (noMeta OptNoMetadataSupport) apply(dev *device) {
	dev.noMetadata = bool(noMeta)
}

//OptThickProvisionLocalFiles instructs an ObjectStore device to not use sparse
// files for local caching
type OptThickProvisionLocalFiles bool

func (thickProv OptThickProvisionLocalFiles) apply(dev *device) {
	dev.thickProvision = bool(thickProv)
}

//OptEncrypt instructs an ObjectStore device to use the provided AES encryption mode
type OptEncrypt struct {
	Mode encrypt.Mode
	Key  []byte
}

func (optEncrypt OptEncrypt) apply(dev *device) {
	dev.encryptMode = optEncrypt.Mode
	dev.encryptKey = optEncrypt.Key
}

//OptQuotaBytes instructs an ObjectStore device to limit the cache backed local disk
// to an amount less than the full size of the device
type OptQuotaBytes int64

func (quota OptQuotaBytes) apply(dev *device) {
	dev.quotaBytes = int64(quota)
}

//OptPersistCache instructs an ObjectStore device to not clean up its local disk cache
// files. Primarily for debugging, but also a performance optimization if no other
// machines are allowed to mutate the remote objects.
type OptPersistCache bool

func (keepUseCache OptPersistCache) apply(dev *device) {
	dev.persistCache = bool(keepUseCache)
}
