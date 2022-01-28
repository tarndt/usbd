package testutil

import (
	"context"
	"fmt"
	"os"
	"path"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tarndt/usbd/pkg/devices/filedisk"
	"github.com/tarndt/usbd/pkg/usbdlib"
)

//TestUserspace runs a suite of basic sanity tests on the provided device directly
// without using Linux kernel facilities such as NBD
func TestUserspace(t *testing.T, dev usbdlib.Device, expectedSize uint) {
	t.Run("user-space", func(t *testing.T) {
		t.Parallel()

		if _, err := dev.ReadAt([]byte{0}, 0); err != nil {
			t.Fatalf("1 byte test read to %#v failed: %s", dev, err)
		}

		TestDevSize(t, dev, expectedSize)
		TestReadEmpty(t, dev, uint(dev.Size()))

		if _, err := dev.WriteAt([]byte{0}, 0); err != nil {
			t.Fatalf("1 byte test write to %#v failed: %s", dev, err)
		}

		TestReadHash(t, dev, TestWriteReadPattern(t, dev))
		TestClose(t, dev)
	})
}

//TestNBD runs a similar test suite as TestUserspace but puts kernel-space NBD
// in the middle for an end-to-end test. This requires privilege execution!
func TestNBD(t *testing.T, dev usbdlib.Device, expectedSize uint) {
	t.Run("with-nbd", func(t *testing.T) {
		t.Parallel()

		const rootUID = 0
		if os.Geteuid() != rootUID {
			pkg := pkgName(dev)
			t.Skipf("Must be root for this test, try: go test -c [-race] && sudo ./%s.test -test.v -test.timeout=120s && rm ./%s.test", pkg, pkg)
		}

		ctx, cancel := context.WithCancel(CreateContext(t))
		defer cancel()

		ndbStream, devPath, err := usbdlib.NewNbdHandler(ctx, dev)
		if err != nil {
			t.Fatalf("Could not find a usable NBD: %s", err)
		}
		t.Logf("Using NBD: %q", devPath)

		f, err := os.OpenFile(devPath, os.O_RDWR, 0)
		if err != nil {
			t.Skipf("Could not open NBD file %q: %s", devPath, err)
		}
		defer f.Close()
		devFile := &filedisk.FileDisk{File: f, SizeBytes: int64(expectedSize)}

		var processingStarted, processingDone sync.WaitGroup
		processingStarted.Add(1)
		processingDone.Add(1)
		var processErr error
		go func() {
			defer processingDone.Done()

			processingStarted.Done()
			err := ndbStream.ProcessRequests()
			if err != nil {
				cancel()
				processErr = fmt.Errorf("Request processing failed: %w", err)
			}
		}()

		processingStarted.Wait()
		time.Sleep(time.Millisecond * 10)

		if _, err := f.ReadAt([]byte{0}, 0); err != nil {
			t.Fatalf("1 byte test read to %q failed: %s", devPath, err)
		}
		TestDevSize(t, devFile, expectedSize)
		TestReadEmpty(t, devFile, uint(devFile.Size()))

		if _, err := f.WriteAt([]byte{0}, 0); err != nil {
			t.Fatalf("1 byte test write to %q failed: %s", devPath, err)
		}
		TestReadHash(t, devFile, TestWriteReadPattern(t, devFile))

		t.Run("shutdown", func(t *testing.T) {
			if err := ndbStream.Close(); err != nil {
				t.Fatalf("Failed to close NBD stream: %s", err)
			}
			processingDone.Wait()
			if err := processErr; err != nil {
				t.Fatalf("Errors occurred while processing NBD stream: %s", err)
			}
		})
	})
}

func pkgName(x interface{}) string {
	varType := strings.TrimPrefix(fmt.Sprintf("%T", x), "*")
	return strings.TrimSuffix(varType, path.Ext(varType))
}
