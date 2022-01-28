package usbdlib_test

import (
	"testing"

	"github.com/tarndt/usbd/pkg/devices/ramdisk"
	"github.com/tarndt/usbd/pkg/devices/testutil"
)

func TestReferenceImpl(t *testing.T) {
	const sizeBytes = 1024 * 1024 * 8
	testutil.TestUserspace(t, ramdisk.NewRAMDisk(1024*1024*8), sizeBytes)
}
