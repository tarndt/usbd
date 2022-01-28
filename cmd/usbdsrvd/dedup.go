package main

import (
	"fmt"
	"path/filepath"

	"github.com/tarndt/usbd/cmd/usbdsrvd/conf"
	"github.com/tarndt/usbd/pkg/devices/dedupdisk"
	"github.com/tarndt/usbd/pkg/devices/dedupdisk/impls"
	"github.com/tarndt/usbd/pkg/usbdlib"
)

func dedupDiskFromCfg(cfg *conf.Config) (usbdlib.Device, error) {
	blockSize := new(usbdlib.DefaultBlockSize).BlockSize()

	lunMap, err := impls.NewMmapLUNmap(filepath.Join(cfg.StorageDirectory, cfg.StorageName+".map"), blockSize, int64(cfg.StorageBytes))
	if err != nil {
		return nil, fmt.Errorf("dedupDiskFromCfg: Could not create LUN map; Details: %w", err)
	}

	idStore, err := impls.NewPebbleIDStore(filepath.Join(cfg.StorageDirectory, cfg.StorageName+".ids"), int(cfg.DedupConfig.IDStoreMemoryCacheBytes), 5)
	if err != nil {
		return nil, fmt.Errorf("dedupDiskFromCfg: Could not create ID store; Details: %w", err)
	}

	blockStore, err := impls.NewFileBlockStore(filepath.Join(cfg.StorageDirectory, cfg.StorageName+".blks"), blockSize)
	if err != nil {
		return nil, fmt.Errorf("dedupDiskFromCfg: Could not create block store; Details: %w", err)
	}

	return dedupdisk.NewDedupDisk(lunMap, idStore, blockStore), nil
}
