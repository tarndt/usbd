package filedisk_test //this is in its own pkg due to testhelpers importing filedisk, this avoid circ dep

import (
	"path/filepath"
	"testing"

	"github.com/tarndt/usbd/pkg/devices/filedisk"
	"github.com/tarndt/usbd/pkg/devices/testutil"
	"github.com/tarndt/usbd/pkg/usbdlib"
)

func TestFileDisk(t *testing.T) {
	const sizeBytes = 128 * 1024 * 1024 //128 MB

	testutil.TestUserspace(t, createDevice(t, sizeBytes), sizeBytes)
	testutil.TestNBD(t, createDevice(t, sizeBytes), sizeBytes)
}

func createDevice(t *testing.T, sizeBytes uint) usbdlib.Device {
	dev, err := filedisk.NewFileDisk(filepath.Join(t.TempDir(), "test.bin"), int64(sizeBytes))
	if err != nil {
		t.Fatalf("Could not create file backed test device: %s", err)
	}
	return dev
}
