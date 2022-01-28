package conf

import (
	"strings"
)

//These are enums that map to each of the available device implementations
const (
	DevUnknown BackingDevice = iota
	DevMem
	DevFile
	DevDedupFile
	DevObjStore
)

//BackingDevice type represents the available device implementations
type BackingDevice uint8

//NewBackingDevice constructs a BackingDevice from a human textual short name (from config)
func NewBackingDevice(devDesc string) BackingDevice {
	switch strings.ToLower(devDesc) {
	case "mem", "memory":
		return DevMem
	case "file", "disk":
		return DevFile
	case "dedup", "dedup-file", "dedup-disk":
		return DevDedupFile
	case "osbd", "objstore":
		return DevObjStore
	default:
		return DevUnknown
	}
}

//String is a human readable description of the device for display
func (bd BackingDevice) String() string {
	switch bd {
	case DevMem:
		return "ramdisk"
	case DevFile:
		return "filedisk"
	case DevDedupFile:
		return "deduplicated-filedisk"
	case DevObjStore:
		return "locally-cached objectstore"
	default:
		return "unknown"
	}
}
