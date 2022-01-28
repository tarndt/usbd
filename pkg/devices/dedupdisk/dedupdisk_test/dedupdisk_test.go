package dedupdisk_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/tarndt/usbd/pkg/devices/dedupdisk"
	"github.com/tarndt/usbd/pkg/devices/dedupdisk/impls"
	"github.com/tarndt/usbd/pkg/devices/testutil"
	"github.com/tarndt/usbd/pkg/usbdlib"
	"github.com/tarndt/usbd/pkg/util/consterr"
)

func TestDedupDisk(t *testing.T) {
	const sizeBytes = 128 * 1024 * 1024 //128 MB

	for _, bs := range blockStoreConstructors() {
		t.Run("with-"+bs.name, func(t *testing.T) {
			testutil.TestUserspace(t, createDevice(t, sizeBytes, bs.newBlockStore), sizeBytes)
			testutil.TestNBD(t, createDevice(t, sizeBytes, bs.newBlockStore), sizeBytes)
		})
	}
}

type blockStoreConstructor func(filename string, blockSize int64) (dedupdisk.BlockStore, error)

func createDevice(t *testing.T, sizeBytes uint, newBlockStore blockStoreConstructor) usbdlib.Device {
	cacheSize := sizeBytes / 4
	if cacheSize < 1 {
		cacheSize = sizeBytes
	}
	blockSize := new(usbdlib.DefaultBlockSize).BlockSize()
	storeDir := t.TempDir()

	lunMap, err := impls.NewMmapLUNmap(filepath.Join(storeDir, "test.map"), blockSize, int64(sizeBytes))
	if err != nil {
		t.Fatalf("Could not create LUN map; Details: %s", err)
	}

	idStore, err := impls.NewPebbleIDStore(filepath.Join(storeDir, "test.ids"), int(cacheSize), 5)
	if err != nil {
		t.Fatalf("Could not create ID store; Details: %s", err)
	}

	blockStore, err := newBlockStore(filepath.Join(storeDir, "test.blks"), blockSize)
	if err != nil {
		if errors.Is(err, consterr.ErrNotImplemented) {
			t.Skip("Skipping expirmental block store")
		}
		t.Fatalf("Could not create block store; Details: %s", err)
	}

	return dedupdisk.NewDedupDisk(lunMap, idStore, blockStore)
}

type blockStore struct {
	newBlockStore blockStoreConstructor
	name          string
}

func blockStoreConstructors() []blockStore {
	return []blockStore{
		blockStore{
			newBlockStore: func(filename string, blockSize int64) (dedupdisk.BlockStore, error) {
				return impls.NewFileBlockStore(filename, blockSize)
			},
			name: "FileBlockStore",
		},
		blockStore{
			newBlockStore: func(filename string, blockSize int64) (dedupdisk.BlockStore, error) {
				return impls.NewLZ4BlockStore(filename, blockSize)
			},
			name: "LZ4BlockStore",
		},
	}
}
