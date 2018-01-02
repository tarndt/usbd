package main

import (
	"flag"
	"log"
	"os"
	"strings"
)

const (
	devUnknown = iota
	devMem
	devFile
	devDedupFile
)

type BackingDevice uint8

func NewBackingDevice(devDesc string) BackingDevice {
	switch strings.ToLower(devDesc) {
	case "mem", "memory":
		return devMem
	case "file", "disk":
		return devFile
	case "dedup", "dedup-file", "dedup-disk":
		return devDedupFile
	default:
		return devUnknown
	}
}

type Config struct {
	NBDDevName       string
	BackingMode      BackingDevice
	StorageDirectory string
	StorageName      string
	StorageGB        int64
}

func mustGetConfig() *Config {
	var devDesc string
	cfg := new(Config)
	flag.StringVar(&devDesc, "dev-type", "mem", "Type of device to back block device with: 'mem', 'file', 'dedup'.")
	flag.StringVar(&cfg.StorageDirectory, "store-dir", "./", "Location to create new backing disk files in")
	flag.StringVar(&cfg.StorageName, "store-name", "test-lun", "File base name to use for new backing disk files")
	flag.Int64Var(&cfg.StorageGB, "sizeGB", 1, "GB storage to use for new backing disk files")

	flag.Parse()
	args := flag.Args()
	if len(args) != 2 {
		flag.PrintDefaults()
		log.Fatalf("Usage: %s <NDB device name>", os.Args[0])
	}
	cfg.NBDDevName = args[0]

	if cfg.BackingMode = NewBackingDevice(devDesc); cfg.BackingMode == devUnknown {
		flag.PrintDefaults()
		log.Fatalf("Bad argument: Unknown backing device type of: %q", devDesc)
	}
	return cfg
}
