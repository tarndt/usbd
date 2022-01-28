package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/tarndt/usbd/cmd/usbdsrvd/conf"
	"github.com/tarndt/usbd/pkg/devices/objstore"
	"github.com/tarndt/usbd/pkg/devices/objstore/compress"
	"github.com/tarndt/usbd/pkg/devices/objstore/encrypt"
	"github.com/tarndt/usbd/pkg/usbdlib"

	"github.com/graymeta/stow"
)

func osbdFromCfg(cfg *conf.Config) (usbdlib.Device, error) {
	oscfg := cfg.ObjStoreConfig

	store, err := objstore.NewStore(oscfg.Kind, oscfg.Config)
	if err != nil {
		return nil, fmt.Errorf("Could not create s3 object store: %w", err)
	}

	container, err := store.Container(cfg.StorageName)
	if err != nil {
		if !errors.Is(err, stow.ErrNotFound) {
			return nil, fmt.Errorf("Could not open remote container %q: %w", cfg.StorageName, err)
		}
		if container, err = store.CreateContainer(cfg.StorageName); err != nil {
			return nil, fmt.Errorf("Could not create remote container %q: %w", cfg.StorageName, err)
		}
	}

	opts := []objstore.Option{objstore.OptConcurFlushCount(oscfg.ConcurFlush)}
	if oscfg.LocalDiskCacheBytes > 0 {
		opts = append(opts, objstore.OptQuotaBytes(oscfg.LocalDiskCacheBytes))
	}
	if oscfg.AESMode != encrypt.ModeIdentity {
		opts = append(opts, objstore.OptEncrypt{Mode: oscfg.AESMode, Key: oscfg.AESKey})
	}
	if oscfg.CompressMode != compress.ModeIdentity {
		opts = append(opts, objstore.OptCompressRemoteObjects(oscfg.CompressMode))
	}
	if oscfg.FlushInterval > 0 {
		opts = append(opts, objstore.OptAutoflushIval(oscfg.FlushInterval))
	}
	if !objstore.SupportsMetaData(oscfg.Kind) {
		opts = append(opts, objstore.OptNoMetadataSupport(true))
	}

	return objstore.NewDevice(
		context.Background(), container, cfg.StorageDirectory,
		uint(cfg.StorageBytes), uint(oscfg.ObjectBytes), opts...,
	)
}
