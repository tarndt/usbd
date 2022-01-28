package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/tarndt/usbd/cmd/usbdsrvd/conf"
	"github.com/tarndt/usbd/pkg/devices/filedisk"
	"github.com/tarndt/usbd/pkg/devices/ramdisk"
	"github.com/tarndt/usbd/pkg/usbdlib"
)

var deamonName = fmt.Sprintf("USBD Server (%s)", os.Args[0])

//Simple usage: go build && sudo ./usbdsrv
func main() {
	cfg := conf.MustGetConfig()

	log.Println(deamonName + " started.")
	log.Printf(deamonName+" using config: %s", cfg)
	defer log.Println(deamonName + " terminated normally.")

	device := mustGetDevice(cfg)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer cancel()

	var (
		ndbStream *usbdlib.NbdStream
		err       error
	)
	if cfg.NBDDevName != "" {
		if ndbStream, _, err = usbdlib.NewNbdHandler(ctx, device, cfg.NBDDevName); err != nil {
			err = fmt.Errorf("Could not use existing NBD device %q: %w", cfg.NBDDevName, err)
		}
	} else {
		if ndbStream, cfg.NBDDevName, err = usbdlib.NewNbdHandler(ctx, device, cfg.NBDDevCount); err != nil {
			err = fmt.Errorf("Could not create new NBD device: %w", err)
		} else {
			defer func() {
				if err := os.Remove(cfg.NBDDevName); err != nil {
					log.Printf("Warning: Could not remove autocreated NBD device %q: %s", cfg.NBDDevName, err)
				}
			}()
		}
	}
	if err != nil {
		log.Fatalf("Could not create NDB user-space device: %s", err)
	}

	log.Printf(deamonName+" is processing requests for %q.", cfg.NBDDevName)
	if err := ndbStream.ProcessRequests(); err != nil {
		log.Fatalf("Request processing failed: %s", err)
	}
}

func mustGetDevice(cfg *conf.Config) (device usbdlib.Device) {
	size := int64(cfg.StorageBytes)
	var err error

	switch cfg.BackingMode {
	case conf.DevMem:
		device = ramdisk.NewRAMDisk(size)

	case conf.DevFile:
		device, err = filedisk.NewFileDisk(filepath.Join(cfg.StorageDirectory, cfg.StorageName+".bin"), size)
		if err != nil {
			log.Fatalf("Could not create file backed virtual disk: %s", err)
		}

	case conf.DevDedupFile:
		device, err = dedupDiskFromCfg(cfg)
		if err != nil {
			log.Fatalf("Could not create pebbleDB backed deduplicating virtual disk: %s", err)
		}

	case conf.DevObjStore:
		device, err = osbdFromCfg(cfg)
		if err != nil {
			log.Fatalf("Could not create object storage backed virtual disk: %s", err)
		}

	default:
		log.Fatalf("Bug: Could not create store: unknown backing device mode enum: %d", cfg.BackingMode)
	}

	return device
}
