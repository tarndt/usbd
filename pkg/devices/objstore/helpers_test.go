package objstore

import (
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/tarndt/usbd/pkg/devices/testutil"
	"github.com/tarndt/usbd/pkg/usbdlib"

	"github.com/graymeta/stow"
	"github.com/graymeta/stow/s3"
	"github.com/johannesboyne/gofakes3"
	"github.com/johannesboyne/gofakes3/backend/s3mem"
)

func s3Server() *httptest.Server {
	return httptest.NewServer(gofakes3.New(s3mem.New()).Server())
}

func s3Store(t *testing.T, srv *httptest.Server) stow.Location {
	cfg := stow.ConfigMap{
		s3.ConfigEndpoint:    srv.URL,
		s3.ConfigAccessKeyID: "fake",
		s3.ConfigSecretKey:   "fake",
	}
	if err := ValidateConfig(KindS3, cfg); err != nil {
		t.Fatalf("Could not validate store config: %s", err)
	}

	store, err := NewStore(KindS3, cfg)
	if err != nil {
		t.Fatalf("Could not create local object store: %s", err)
	}

	return store
}

func createContainer(t *testing.T, store stow.Location) stow.Container {
	container, err := store.CreateContainer(strconv.FormatInt(time.Now().UnixNano(), 36))
	if err != nil {
		t.Fatalf("Could not create container: %s", err)
	}
	return container
}

func createDevice(t *testing.T, container stow.Container, optCacheDir string, totalBytes, objectBytes uint, options ...Option) usbdlib.Device {
	if optCacheDir == "" {
		optCacheDir = t.TempDir()
	}

	dev, err := NewDevice(testutil.CreateContext(t), container, optCacheDir, totalBytes, objectBytes, options...)
	if err != nil {
		t.Fatalf("Could not create device: %s", err)
	}
	return dev
}

func testExistingRemote(t *testing.T, container stow.Container, optReUseCacheDir string, totalBytes, objectBytes uint, expectedHash []byte, options ...Option) {
	t.Run("existing-remote-data", func(t *testing.T) {
		dev := createDevice(t, container, optReUseCacheDir, totalBytes, objectBytes, options...)
		testutil.TestReadHash(t, dev, expectedHash)
		if dev.Size() > 0 { //Double check device is still writable
			if _, err := dev.WriteAt([]byte{7}, 0); err != nil {
				t.Fatalf("Reconstructed device was not still writable: %s", err)
			}
		}
		testutil.TestClose(t, dev)
	})
}
