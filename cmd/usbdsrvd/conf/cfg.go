package conf

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/tarndt/usbd/pkg/devices/objstore/compress"
	"github.com/tarndt/usbd/pkg/devices/objstore/encrypt"

	"github.com/dustin/go-humanize"
	"github.com/graymeta/stow"
)

//Config is a representation of command line config parameters
type Config struct {
	NBDDevName       string
	NBDDevCount      uint
	BackingMode      BackingDevice
	StorageDirectory string
	StorageName      string
	StorageBytes     Capacity
	DedupConfig
	ObjStoreConfig
}

//String generates human-readable prose describing a configuration
func (cfg *Config) String() string {
	devName := "next available NBD device"
	if cfg.NBDDevName != "" {
		devName = cfg.NBDDevName
	}

	driverParams := ""
	switch cfg.BackingMode {
	case DevDedupFile:
		driverParams = " " + cfg.DedupConfig.String()
	case DevObjStore:
		driverParams = " " + cfg.ObjStoreConfig.String()
	}

	return fmt.Sprintf("Exporting %s volume %q as %s with local storage at %q using driver %s%s.",
		humanize.IBytes(uint64(cfg.StorageBytes)), cfg.StorageName,
		devName, cfg.StorageDirectory, cfg.BackingMode, driverParams,
	)
}

//DedupConfig is the dedup-disk specific configuration parameters
type DedupConfig struct {
	IDStoreMemoryCacheBytes int64
}

//String generates human-readable prose describing a DedupConfig
func (dc *DedupConfig) String() string {
	return fmt.Sprintf("using up to %s memory for ID Store cache", humanize.IBytes(uint64(dc.IDStoreMemoryCacheBytes)))
}

//ObjStoreConfig is the objectstore specific configuration parameters
type ObjStoreConfig struct {
	Kind                string
	Config              stow.ConfigMap
	LocalDiskCacheBytes Capacity
	ObjectBytes         Capacity
	AESMode             encrypt.Mode
	AESKey              []byte
	CompressMode        compress.Mode
	ConcurFlush         uint
	FlushInterval       time.Duration
}

//String generates human-readable prose describing a ObjStoreConfig
func (c *ObjStoreConfig) String() string {
	size := "as much as total device size"
	if c.LocalDiskCacheBytes > 0 {
		size = humanize.IBytes(uint64(c.LocalDiskCacheBytes))
	}

	var flushDesc string
	if c.FlushInterval > 0 {
		flushDesc = fmt.Sprintf(", flushing to remote story every %s using %d workers", c.FlushInterval, c.ConcurFlush)
	}

	return fmt.Sprintf("using a %s remote object store (%s), with %s objects, using up to %s of local storage, %s compression, %s encryption%s",
		c.Kind, stowCfgStr(c.Config), humanize.IBytes(uint64(c.ObjectBytes)),
		size, c.CompressMode, c.AESMode, flushDesc,
	)
}

func stowCfgStr(cm stow.ConfigMap) string {
	var str bytes.Buffer
	for k, v := range cm {
		str.WriteString(k)
		str.WriteByte('=')

		if mayBeSecret(k) {
			str.WriteString("<REDACTED>")
		} else {
			str.WriteByte('"')
			str.WriteString(v)
			str.WriteByte('"')
		}
		str.WriteString(", ")
	}
	if str.Len() > 2 {
		str.Truncate(str.Len() - 2)
	}
	return str.String()
}

func mayBeSecret(s string) bool {
	s = strings.ToLower(s)
	for _, cannidate := range []string{"secret", "cred", "pass", "token"} {
		if strings.Contains(s, cannidate) {
			return true
		}
	}
	return false
}
