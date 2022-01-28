package compress

import (
	"fmt"
	"io"
	"strconv"

	"github.com/tarndt/usbd/pkg/util/strms"

	"github.com/graymeta/stow"
)

const (
	compressMetaAlgoHeader = "x-osbd-cmp-alg"
	compressMetaSizeHeader = "x-osbd-cmp-size"
)

type compressedContainer struct {
	stow.Container
	Mode
}

var _ stow.Container = (*compressedContainer)(nil)

//NewCompressedContainer wraps the provided container in transparent compression and decompression
func NewCompressedContainer(container stow.Container, mode Mode) stow.Container {
	return compressedContainer{
		Container: container,
		Mode:      mode,
	}
}

func (cont compressedContainer) Item(id string) (stow.Item, error) {
	item, err := cont.Container.Item(id)
	if item != nil {
		item = compressedItem{item}
	}
	return item, err
}

func (cont compressedContainer) Items(prefix, cursor string, count int) ([]stow.Item, string, error) {
	items, cursor, err := cont.Container.Items(prefix, cursor, count)
	for i := range items {
		items[i] = compressedItem{items[i]}
	}
	return items, cursor, err
}

func (cont compressedContainer) Put(name string, rdr io.Reader, size int64, metadata map[string]interface{}) (stow.Item, error) {
	if cont.Mode == ModeIdentity {
		return cont.Container.Put(name, rdr, size, metadata)
	}

	pipeRdr, pipeWtr := io.Pipe()
	wtr, err := cont.NewWriter(pipeWtr)
	if err != nil {
		return nil, fmt.Errorf("Could not create stream compressor: %w", err)
	}

	go func() {
		_, err := io.Copy(wtr, rdr)
		if err != nil {
			pipeWtr.CloseWithError(fmt.Errorf("Copy failed during %s stream compression: %w", cont.Mode, err))
			wtr.Close()
			return
		}

		if err = wtr.Close(); err != nil {
			pipeWtr.CloseWithError(fmt.Errorf("Close failed during %s stream compression: %w", cont.Mode, err))
			return
		}

		pipeWtr.Close()
	}()

	mdWithCmp := make(map[string]interface{}, len(metadata)+2)
	for key, val := range metadata {
		mdWithCmp[key] = val
	}
	mdWithCmp[compressMetaAlgoHeader] = cont.AlgoName()
	mdWithCmp[compressMetaSizeHeader] = strconv.FormatInt(size, 36)

	return cont.Container.Put(name, pipeRdr, stow.SizeUnknown, mdWithCmp)
}

type compressedItem struct {
	stow.Item
}

var _ stow.Item = (*compressedItem)(nil)

func (item compressedItem) Size() (int64, error) {
	md, err := item.Metadata()
	if err != nil {
		return 0, fmt.Errorf("Opening item metadata to check for compression failed: %w", err)
	}

	hdrAlgoVal, hdrExists := md[compressMetaAlgoHeader]
	if !hdrExists {
		return item.Item.Size()
	}

	hdrSizeVal, hdrExists := md[compressMetaSizeHeader]
	if !hdrExists {
		return 0, fmt.Errorf("Item metadata indicated (%v) compression but no size was recorded", hdrAlgoVal)
	}

	hdrSizeValStr, isString := hdrSizeVal.(string)
	if !isString {
		return 0, fmt.Errorf("Item metadata indicated (%v) compression but size (#%v) was a %T not a string", hdrAlgoVal, hdrSizeVal, hdrSizeVal)
	}

	size, err := strconv.ParseInt(hdrSizeValStr, 36, 64)
	if err != nil {
		return 0, fmt.Errorf("Item metadata size %q was not parseable as base36 integer: %w", hdrSizeValStr, err)
	}
	return size, nil
}

func (item compressedItem) Open() (io.ReadCloser, error) {
	md, err := item.Metadata()
	if err != nil {
		return nil, fmt.Errorf("Opening item metadata to check for compression failed: %w", err)
	}

	mode := ModeIdentity
	if hdrAlgoVal, hdrExists := md[compressMetaAlgoHeader]; hdrExists {
		hdrAlgoStr, isString := hdrAlgoVal.(string)
		if !isString {
			return nil, fmt.Errorf("Item metadata indicated compression but value (#%v) was a %T not a string", hdrAlgoVal, hdrAlgoVal)
		}

		if mode = ModeFromName(hdrAlgoStr); mode == ModeUnknown {
			return nil, fmt.Errorf("Item metadata specified unsupported compression mode: %q", hdrAlgoStr)
		}
	}

	itemRdr, err := item.Item.Open()
	if mode == ModeIdentity || err != nil {
		return itemRdr, err
	}
	decompRdr, err := mode.NewReader(itemRdr)
	if err != nil {
		itemRdr.Close()
		return nil, fmt.Errorf("Item decompressor could not be created: %w", err)
	}
	return strms.NewReadFirstCloseList(decompRdr, itemRdr), nil
}
