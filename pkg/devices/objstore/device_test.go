package objstore

import (
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/tarndt/usbd/pkg/devices/objstore/compress"
	"github.com/tarndt/usbd/pkg/devices/objstore/encrypt"
	"github.com/tarndt/usbd/pkg/devices/testutil"
	"github.com/tarndt/usbd/pkg/usbdlib"

	"github.com/graymeta/stow"
	"github.com/graymeta/stow/local"
	"github.com/graymeta/stow/s3"
	"github.com/johannesboyne/gofakes3"
	"github.com/johannesboyne/gofakes3/backend/s3mem"
)

const testDirPrefix = "osbd-test-"

func TestNewDevices(t *testing.T) {
	testLocalFS(t)
	testLocalS3(t)
}

func testLocalFS(t *testing.T) {
	t.Run("local-fs", func(t *testing.T) {
		t.Parallel()

		cfg := stow.ConfigMap{local.ConfigKeyPath: t.TempDir()}
		if err := ValidateConfig(local.Kind, cfg); err != nil {
			t.Fatalf("Could not validate store config: %s", err)
		}

		store, err := NewStore(KindLocalTest, cfg)
		if err != nil {
			t.Fatalf("Could not create s3 object store: %s", err)
		}

		t.Run("no-quota", func(t *testing.T) {
			testDeviceSizes(t, store, OptNoMetadataSupport(true))
		})
		t.Run("with-quota", func(t *testing.T) {
			testDeviceSizes(t, store, OptQuotaBytes(32*1024*1024))
		})
	})
}

func testLocalS3(t *testing.T) {
	t.Run("local-s3", func(t *testing.T) {
		t.Parallel()

		srv := httptest.NewServer(gofakes3.New(s3mem.New()).Server())
		defer srv.Close()

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

		t.Run("compress-none", func(t *testing.T) {
			testDeviceSizes(t, store)
		})
		t.Run("compress-s2", func(t *testing.T) {
			testDeviceSizes(t, store, OptCompressRemoteObjects(compress.ModeS2))
		})
		t.Run("compress-gz", func(t *testing.T) {
			testDeviceSizes(t, store, OptCompressRemoteObjects(compress.ModeGzip))
		})
		t.Run("encrypt", func(t *testing.T) {
			key, err := encrypt.MakeRandomAESKey()
			if err != nil {
				t.Fatalf("Could not create AES key: %s", err)
			}
			t.Run("compress-none", func(t *testing.T) {
				testDeviceSizes(t, store, OptEncrypt{Mode: encrypt.ModeAESRec, Key: key})
			})
			t.Run("compress-s2", func(t *testing.T) {
				testDeviceSizes(t, store, OptCompressRemoteObjects(compress.ModeS2), OptEncrypt{Mode: encrypt.ModeAESRec, Key: key})
			})
		})
	})
}

func testDeviceSizes(t *testing.T, store stow.Location, options ...Option) {
	t.Run("mono-object", func(t *testing.T) {
		const (
			totalBytes  = 2 * 1024 * 1024 //2 MB
			objectBytes = totalBytes
		)
		t.Run("thick", func(t *testing.T) {
			testSuite(t, store, totalBytes, objectBytes, append(options, OptAutoflushIval(time.Millisecond*50), OptThickProvisionLocalFiles(true))...)
		})
		t.Run("thin", func(t *testing.T) {
			testSuite(t, store, totalBytes, objectBytes, append(options, OptAutoflushIval(time.Millisecond*50))...)
		})
	})

	t.Run("micro-objects", func(t *testing.T) {
		const (
			totalBytes  = 16 * 1024 * 1024 //16 MB
			objectBytes = 512 * 1024       //512 KB
		)
		testSuite(t, store, totalBytes, objectBytes, options...)
	})

	if testing.Short() {
		t.Skipf("In short mode, skipping larger tests")
	}

	t.Run("small-objects", func(t *testing.T) {
		const (
			totalBytes  = 64 * 1024 * 1024 //64 MB
			objectBytes = 2 * 1024 * 1024  //2 MB
		)

		testSuite(t, store, totalBytes, objectBytes, append(options, OptConcurFlushCount(2))...)
	})

	t.Run("medium-objects", func(t *testing.T) {
		const (
			totalBytes  = 256 * 1024 * 1024 //256 MB
			objectBytes = 16 * 1024 * 1024  //16 MB
		)

		testSuite(t, store, totalBytes, objectBytes, append(options, OptConcurFlushCount(totalBytes/objectBytes))...)
	})
}

func testSuite(t *testing.T, store stow.Location, totalBytes, objectBytes uint, options ...Option) {
	t.Parallel()

	container := createContainer(t, store)
	dev := createDevice(t, container, totalBytes, objectBytes, options...)

	testutil.TestDevSize(t, dev, totalBytes)
	testutil.TestReadEmpty(t, dev, objectBytes)
	devHash := testutil.TestWriteReadPattern(t, dev)
	testExistingRemote(t, container, totalBytes, objectBytes, devHash, options...)
	testutil.TestClose(t, dev)
}

func createContainer(t *testing.T, store stow.Location) stow.Container {
	container, err := store.CreateContainer(strconv.FormatInt(time.Now().UnixNano(), 36))
	if err != nil {
		t.Fatalf("Could not create container: %s", err)
	}
	return container
}

func createDevice(t *testing.T, container stow.Container, totalBytes, objectBytes uint, options ...Option) usbdlib.Device {
	dev, err := NewDevice(testutil.CreateContext(t), container, t.TempDir(), totalBytes, objectBytes, options...)
	if err != nil {
		t.Fatalf("Could not create device: %s", err)
	}
	return dev
}

func testExistingRemote(t *testing.T, container stow.Container, totalBytes, objectBytes uint, expectedHash []byte, options ...Option) {
	t.Run("existing-remote-data", func(t *testing.T) {
		dev := createDevice(t, container, totalBytes, objectBytes, options...)
		testutil.TestReadHash(t, dev, expectedHash)
	})
}
