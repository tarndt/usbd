package objstore

import (
	"testing"
	"time"

	"github.com/tarndt/usbd/pkg/devices/objstore/compress"
	"github.com/tarndt/usbd/pkg/devices/objstore/encrypt"
	"github.com/tarndt/usbd/pkg/devices/testutil"

	"github.com/graymeta/stow"
	"github.com/graymeta/stow/local"
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

		srv := s3Server()
		defer srv.Close()

		store := s3Store(t, srv)

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
	dev := createDevice(t, container, "", totalBytes, objectBytes, options...)

	testutil.TestDevSize(t, dev, totalBytes)
	testutil.TestReadEmpty(t, dev, objectBytes)
	devHash := testutil.TestWriteReadPattern(t, dev)
	testExistingRemote(t, container, "", totalBytes, objectBytes, devHash, options...)
	testutil.TestClose(t, dev)
}
