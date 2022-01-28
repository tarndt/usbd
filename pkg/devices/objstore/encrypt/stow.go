package encrypt

import (
	"encoding/hex"
	"fmt"
	"io"
	"strconv"

	"github.com/tarndt/usbd/pkg/util/strms"

	"github.com/graymeta/stow"
)

const (
	encryptMetaAlgoHeader = "x-osbd-crypt-alg"
	encryptMetaIVHeader   = "x-osbd-crypt-iv"
	encryptMetaSizeHeader = "x-osbd-crypt-size"
)

type encryptedContainer struct {
	stow.Container
	Mode
	key []byte
}

var _ stow.Container = (*encryptedContainer)(nil)

//NewEncryptedContainer wraps the provided container in transparent encryption and decryption
func NewEncryptedContainer(container stow.Container, mode Mode, key []byte) stow.Container {
	return encryptedContainer{
		Container: container,
		Mode:      mode,
		key:       key,
	}
}

func (cont encryptedContainer) Item(id string) (stow.Item, error) {
	item, err := cont.Container.Item(id)
	if item != nil {
		item = encryptedItem{Item: item, key: cont.key}
	}
	return item, err
}

func (cont encryptedContainer) Items(prefix, cursor string, count int) ([]stow.Item, string, error) {
	items, cursor, err := cont.Container.Items(prefix, cursor, count)
	for i := range items {
		items[i] = encryptedItem{Item: items[i], key: cont.key}
	}
	return items, cursor, err
}

func (cont encryptedContainer) Put(name string, rdr io.Reader, size int64, metadata map[string]interface{}) (stow.Item, error) {
	if cont.Mode == ModeIdentity {
		return cont.Container.Put(name, rdr, size, metadata)
	}

	pipeRdr, pipeWtr := io.Pipe()
	wtr, initVect, err := cont.NewWriter(pipeWtr, cont.key)
	if err != nil {
		return nil, fmt.Errorf("Could not create stream encryptor: %w", err)
	}

	go func() {
		_, err := io.Copy(wtr, rdr)
		if err != nil {
			pipeWtr.CloseWithError(fmt.Errorf("Copy failed during %s stream encryption: %w", cont.Mode, err))
			wtr.Close()
			return
		}

		if err = wtr.Close(); err != nil {
			pipeWtr.CloseWithError(fmt.Errorf("Close failed during %s stream encryption: %w", cont.Mode, err))
			return
		}

		pipeWtr.Close()
	}()

	mdWithCrypt := make(map[string]interface{}, len(metadata)+2)
	for key, val := range metadata {
		mdWithCrypt[key] = val
	}
	mdWithCrypt[encryptMetaAlgoHeader] = cont.AlgoName()
	mdWithCrypt[encryptMetaIVHeader] = hex.EncodeToString(initVect)
	mdWithCrypt[encryptMetaSizeHeader] = strconv.FormatInt(size, 36)

	return cont.Container.Put(name, pipeRdr, stow.SizeUnknown, mdWithCrypt)
}

type encryptedItem struct {
	stow.Item
	key []byte
}

var _ stow.Item = (*encryptedItem)(nil)

func (item encryptedItem) Size() (int64, error) {
	md, err := item.Metadata()
	if err != nil {
		return 0, fmt.Errorf("Opening item metadata to check for encryption failed: %w", err)
	}

	hdrAlgoVal, hdrExists := md[encryptMetaAlgoHeader]
	if !hdrExists {
		return item.Item.Size()
	}

	hdrSizeVal, hdrExists := md[encryptMetaSizeHeader]
	if !hdrExists {
		return 0, fmt.Errorf("Item metadata indicated (%v) encryption but no size was recorded", hdrAlgoVal)
	}

	hdrSizeValStr, isString := hdrSizeVal.(string)
	if !isString {
		return 0, fmt.Errorf("Item metadata indicated (%v) encryption but size (#%v) was a %T not a string", hdrAlgoVal, hdrSizeVal, hdrSizeVal)
	}

	size, err := strconv.ParseInt(hdrSizeValStr, 36, 64)
	if err != nil {
		return 0, fmt.Errorf("Item metadata size %q was not parseable as base36 integer: %w", hdrSizeValStr, err)
	}
	return size, nil
}

func (item encryptedItem) Open() (io.ReadCloser, error) {
	md, err := item.Metadata()
	if err != nil {
		return nil, fmt.Errorf("Opening item metadata to check for encryption failed: %w", err)
	}

	mode := ModeIdentity
	var initVect []byte
	if hdrAlgoVal, hdrExists := md[encryptMetaAlgoHeader]; hdrExists {
		hdrAlgoStr, isString := hdrAlgoVal.(string)
		if !isString {
			return nil, fmt.Errorf("Item metadata indicated encryption but value (#%v) was a %T not a string", hdrAlgoVal, hdrAlgoVal)
		}

		if mode = ModeFromName(hdrAlgoStr); mode == ModeUnknown {
			return nil, fmt.Errorf("Item metadata specified unsupported encryption mode: %q", hdrAlgoStr)
		}

		if hdrInitVecVal, hdrExists := md[encryptMetaIVHeader]; !hdrExists {
			return nil, fmt.Errorf("Item metadata indicated %s encryption but no IV (intialization vector) was recorded", mode)
		} else if hdrInitVecStr, isString := hdrInitVecVal.(string); !isString {
			return nil, fmt.Errorf("Item metadata indicated %s encryption but no IV (intialization vector) (#%v) was a %T not a string", mode, hdrInitVecVal, hdrInitVecVal)
		} else if initVect, err = hex.DecodeString(hdrInitVecStr); err != nil {
			return nil, fmt.Errorf("Item metadata IV (intialization vector) %q was not parseable as base16 value: %w", hdrInitVecStr, err)
		}
	}

	itemRdr, err := item.Item.Open()
	if mode == ModeIdentity || err != nil {
		return itemRdr, err
	}
	decompRdr, err := mode.NewReader(itemRdr, item.key, initVect)
	if err != nil {
		itemRdr.Close()
		return nil, fmt.Errorf("Item decyptor could not be created: %w", err)
	}
	return strms.NewReadFirstCloseList(decompRdr, itemRdr), nil
}
