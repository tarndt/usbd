package objstore

import (
	"io"
	"testing"

	"github.com/tarndt/usbd/pkg/devices/objstore/compress"
	"github.com/tarndt/usbd/pkg/devices/testutil"
	"github.com/tarndt/usbd/pkg/util/consterr"

	"github.com/graymeta/stow"
)

const errNoDownloadAllowed = consterr.ConstErr("Downloads are disabled")

func TestCachepersistence(t *testing.T) {
	t.Parallel()

	srv := s3Server()
	defer srv.Close()

	store := s3Store(t, srv)
	options := []Option{
		OptCompressRemoteObjects(compress.ModeS2),
		OptPersistCache(true),
	}

	const (
		totalBytes  = 8 * 1024 * 1024 //8 MB
		objectBytes = 512 * 1024      //512 KB
	)

	container := createContainer(t, store)
	cacheDir := t.TempDir()
	dev := createDevice(t, container, cacheDir, totalBytes, objectBytes, options...)

	if !dev.(*device).persistCache {
		t.Fatal("Cache persistence is off when it should be on!")
	}

	//Some standard tests
	testutil.TestDevSize(t, dev, totalBytes)
	testutil.TestReadEmpty(t, dev, objectBytes)

	//Test persistence specifically
	devHash := testutil.TestWriteReadPattern(t, dev)
	testutil.TestClose(t, dev)

	//noRemoteDownloadContainer will explode if item.Get is called.
	// This allows to know that all of the items were serviced from local cache
	// and no downloads of remote objects occurred
	testExistingRemote(t, noRemoteDownloadContainer{container}, cacheDir, totalBytes, objectBytes, devHash, options...)
}

type noRemoteDownloadContainer struct {
	stow.Container
}

func (cont noRemoteDownloadContainer) Item(id string) (stow.Item, error) {
	item, err := cont.Container.Item(id)
	if item != nil {
		item = noDownloadItem{item}
	}
	return item, err
}

func (cont noRemoteDownloadContainer) Items(prefix, cursor string, count int) ([]stow.Item, string, error) {
	items, cursor, err := cont.Container.Items(prefix, cursor, count)
	for i := range items {
		items[i] = noDownloadItem{items[i]}
	}
	return items, cursor, err
}

type noDownloadItem struct {
	stow.Item
}

func (item noDownloadItem) Open() (io.ReadCloser, error) {
	return nil, errNoDownloadAllowed
}
