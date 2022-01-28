package ramdisk

import (
	"testing"

	"github.com/tarndt/usbd/pkg/devices/testutil"
)

func TestRamdisk(t *testing.T) {
	const sizeBytes = 128 * 1024 * 1024 //128 MB

	testutil.TestUserspace(t, NewRAMDisk(sizeBytes), sizeBytes)
	testutil.TestNBD(t, NewRAMDisk(sizeBytes), sizeBytes)
}
