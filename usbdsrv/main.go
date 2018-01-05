package main

import (
	"log"
	"path/filepath"

	"github.com/tarndt/usbd/devices/dedupdisk"
	"github.com/tarndt/usbd/devices/dedupdisk/impls"
	"github.com/tarndt/usbd/devices/filedisk"
	"github.com/tarndt/usbd/devices/ramdisk"
	"github.com/tarndt/usbd/usbdlib"
)

const bytes1GB = 1024 * 1024 * 1024

func main() {
	cfg := mustGetConfig()
	device := getDevice(cfg)

	ndbStream, err := usbdlib.NewNbdStream(cfg.NBDDevName, device)
	if err != nil {
		log.Fatalf("Could not create ndbStream; Details: %s", err)
	}

	usbdlib.NewReqProcessor(ndbStream, device, usbdlib.RecommendWorkerCount())
	ndbStream.Done().Wait()
}

func getDevice(cfg *Config) (device usbdlib.Device) {
	size := cfg.StorageGB * bytes1GB

	switch cfg.BackingMode {
	case devMem:
		device = ramdisk.New(size)
	case devFile:
		var err error
		device, err = filedisk.New(filepath.Join(cfg.StorageDirectory, cfg.StorageName+".bin"), size)
		if err != nil {
			log.Fatalf("Could not create virtual disk; Details: %s", err)
		}
	case devDedupFile:
		blockSize := new(usbdlib.DefaultBlockSize).BlockSize()
		lunMap, err := impls.NewMmapLUNmap(filepath.Join(cfg.StorageDirectory, cfg.StorageName+".map"), blockSize, size)
		if err != nil {
			log.Fatalf("Could not create LUN map; Details: %s", err)
		}
		idStore, err := impls.NewRocksIdStore(filepath.Join(cfg.StorageDirectory, cfg.StorageName+".ids"), bytes1GB, 5)
		if err != nil {
			log.Fatalf("Could not create ID store; Details: %s", err)
		}
		blockStore, err := impls.NewLZ4BlockStore(filepath.Join(cfg.StorageDirectory, cfg.StorageName+".blks"), blockSize)
		if err != nil {
			log.Fatalf("Could not create block store; Details: %s", err)
		}
		return dedupdisk.New(lunMap, idStore, blockStore)
	default:
		log.Fatalf("Bug: Could not create store; Details: unknown backing device mode enum: %d", cfg.BackingMode)
	}

	return device
}
