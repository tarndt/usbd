package objstore

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/tarndt/sema"
	"github.com/tarndt/usbd/pkg/util/consterr"
	"github.com/tarndt/usbd/pkg/util/strms"

	"github.com/dustin/go-humanize"
	"github.com/graymeta/stow"
)

const errCapacityClaim = consterr.ConstErr("Maximum number of local segments already loaded")

const bufSize = 1024 * 1024 * 2 //2 MB
var copyBufPool sync.Pool = sync.Pool{
	New: func() interface{} {
		return make([]byte, bufSize)
	},
}

type storeParams struct {
	container                    stow.Container
	segmentBytes                 int64
	cacheDir                     string
	thickProvision, persistCache bool

	quotaSema sema.CountingSema
}

func (sp *storeParams) createFile(segID string) (file *os.File, err error) {
	if err = sp.claimCapacity(); err != nil {
		return nil, fmt.Errorf("Not enough capacity to create file: %w", err)
	}

	itemName := osbdPrefix + devicePrefix + sp.container.Name() + blockPrefix + segID
	fpath := filepath.Join(sp.cacheDir, itemName)
	file, err = os.Create(fpath)
	if err != nil {
		return nil, fmt.Errorf("Could not create new object local file %q: %w", fpath, err)
	}
	defer func() {
		if err != nil {
			file.Close()
			os.Remove(fpath)
		}
	}()

	if sp.thickProvision {
		buf := copyBufPool.Get().([]byte)
		limitedRdr := io.LimitedReader{R: strms.DevZero, N: sp.segmentBytes}
		_, err = io.CopyBuffer(file, &limitedRdr, buf)
		copyBufPool.Put(buf)
		if err != nil {
			return nil, fmt.Errorf("Could not write zeros to new object local file %q: %w", fpath, err)
		}
	} else {
		if _, err = file.WriteAt([]byte{0}, sp.segmentBytes-1); err != nil {
			return nil, fmt.Errorf("Could not allocate new sparse object local file %q: %w", fpath, err)
		}
	}

	return file, err
}

func (sp *storeParams) downloadFile(item stow.Item) (file *os.File, err error) {
	if err = sp.claimCapacity(); err != nil {
		return nil, fmt.Errorf("Not enough capacity to download file: %w", err)
	}
	defer func() {
		if err != nil {
			sp.releaseCapacity()
			if file != nil {
				file.Close()
				os.Remove(file.Name())
			}
		}
	}()

	remoteSize, err := item.Size()
	if err != nil {
		return nil, fmt.Errorf("Could not obtain %s size: %w", describeItem(item), err)
	} else if remoteSize == 0 {
		return nil, nil //we don't have anything to download
	} else if remoteSize != sp.segmentBytes {
		return nil, fmt.Errorf("Store reported %s was wrong size: Expected %s and found: %s ", describeItem(item), humanize.IBytes(uint64(sp.segmentBytes)), humanize.IBytes(uint64(remoteSize)))
	}

	fpath := filepath.Join(sp.cacheDir, item.Name())

	file, err = sp.cachedFile(item, fpath, remoteSize)
	if err != nil {
		return nil, fmt.Errorf("Local cache is unusable for %s: %w", describeItem(item), err)
	} else if file != nil {
		return file, nil
	}

	if file, err = os.Create(fpath); err != nil {
		return nil, fmt.Errorf("Could not create download object local file %q: %w", fpath, err)
	}

	data, err := item.Open()
	if err != nil {
		return nil, fmt.Errorf("Could not open %s for downloading: %w", describeItem(item), err)
	}
	defer data.Close()

	limitedRdr := &io.LimitedReader{R: data, N: remoteSize + 1}
	var downloadedBytes int64
	if sp.thickProvision {
		buf := copyBufPool.Get().([]byte)
		downloadedBytes, err = io.CopyBuffer(file, limitedRdr, buf)
		copyBufPool.Put(buf)
	} else {
		downloadedBytes, err = sparseCopy(file, limitedRdr)
	}

	switch {
	case err != nil:
		return nil, fmt.Errorf("Could not download %s: %w", describeItem(item), err)
	case limitedRdr.N < 1:
		return nil, fmt.Errorf("%s contained more than the expected %d bytes (%s) of data", describeItem(item), remoteSize, humanize.IBytes(uint64(remoteSize)))
	case downloadedBytes != remoteSize:
		return nil, fmt.Errorf("Store download %s was wrong size: Expected %d bytes (%s) and found: %d bytes (%s)", describeItem(item), remoteSize, humanize.IBytes(uint64(remoteSize)), downloadedBytes, humanize.IBytes(uint64(downloadedBytes)))
	}
	return file, nil
}

func (sp *storeParams) cachedFile(item stow.Item, fpath string, remoteSize int64) (*os.File, error) {
	dataFileInfo, err := os.Stat(fpath)
	dataExists := err == nil
	eTagPath := eTagFileName(fpath)
	_, err = os.Stat(eTagPath)
	eTagExists := err == nil

	rmDataFile := func() error {
		if err := os.Remove(fpath); err != nil {
			return fmt.Errorf("Could not remove local cache file %q that did not have eTag metadata in preparation for re-download: %w", fpath, err)
		}
		return nil
	}

	rmETagFile := func() error {
		if err := os.Remove(eTagPath); err != nil {
			return fmt.Errorf("Could not remove local cache eTag metadata file %q that did not have matching local data file in preparation for re-download: %w", eTagPath, err)
		}
		return nil
	}

	switch {
	case dataExists && !eTagExists:
		if err = rmDataFile(); err != nil {
			return nil, err
		}

	case !dataExists && eTagExists:
		if err = rmETagFile(); err != nil {
			return nil, err
		}

	case !sp.persistCache && (dataExists || eTagExists):
		return nil, fmt.Errorf("Local cache and/or eTag metadata files exist but cache persistence is not enabled; Enable persistence or remove local files")

	case dataExists && eTagExists:
		fileETagBytes, err := os.ReadFile(eTagPath)
		if err != nil {
			return nil, fmt.Errorf("Could not read eTag metadata file %q while checking in locally persisted cache file could be used: %w", eTagPath, err)
		}
		fileETag := string(fileETagBytes)

		itemETag, err := item.ETag()
		if err != nil {
			return nil, fmt.Errorf("Could not get remote item %s eTag metadata: %w", describeItem(item), err)
		}

		//Is the local data file valid for re-use?
		if fileETag != "" && fileETag == itemETag && dataFileInfo.Mode().IsRegular() && dataFileInfo.Size() == remoteSize {
			file, err := os.OpenFile(fpath, os.O_RDWR, 0666)
			if err != nil {
				return nil, fmt.Errorf("Could not open locally cached data file %q for re-use: %w", fpath, err)
			}
			return file, nil
		}

		//It was not, get rid of the them
		rmDataErr, rmETagErr := rmDataFile(), rmETagFile()
		switch {
		case rmDataErr != nil:
			return nil, rmDataErr
		case rmETagErr != nil:
			return nil, rmETagErr
		}
	}
	return nil, nil
}

func (sp *storeParams) removeFile(file *os.File) error {
	fpath := file.Name()

	err := file.Close()
	if err != nil {
		return fmt.Errorf("Could not close local cache file %q before deleting it: %w", fpath, err)
	}

	if err = os.Remove(fpath); err != nil {
		return fmt.Errorf("Could not remove local cache file %q: %w", fpath, err)
	}

	if sp.persistCache {
		if err = os.Remove(eTagFileName(fpath)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("Could not remove local file %q: %w", fpath, err)
		}
	}
	return nil
}

func (sp *storeParams) syncFile(file *os.File, segID string, optBuf *bytes.Buffer, optEarlyUnlock func()) (stow.Item, error) {
	if err := file.Sync(); err != nil {
		if optEarlyUnlock != nil {
			optEarlyUnlock()
		}
		return nil, fmt.Errorf("Failed to fsync local file: %w", err)
	}

	var srcRdr io.Reader
	if optEarlyUnlock != nil && optBuf != nil && int64(optBuf.Cap()) >= sp.segmentBytes {
		optBuf.Reset()
		if n, err := optBuf.ReadFrom(strms.NewReadAtReader(file)); err != nil {
			optEarlyUnlock()
			return nil, fmt.Errorf("Could not buffer file for transmission to remote object store: %w", err)
		} else if n != sp.segmentBytes {
			optEarlyUnlock()
			return nil, fmt.Errorf("Fewer bytes (%d) than expected (%d) were read while buffering file for transmission to remote object store", n, sp.segmentBytes)
		}
		optEarlyUnlock()
		srcRdr = optBuf
	} else {
		if optEarlyUnlock != nil {
			defer optEarlyUnlock()
		}
		srcRdr = bufio.NewReader(strms.NewReadAtReader(file))
	}

	itemName := osbdPrefix + devicePrefix + sp.container.Name() + blockPrefix + segID
	item, err := sp.container.Put(itemName, srcRdr, sp.segmentBytes, nil)
	if err != nil {
		return nil, fmt.Errorf("Could not upload to remote item %q in %s: %w", itemName, describeContainer(sp.container), err)
	}

	if sp.persistCache {
		if err = persistEtag(file.Name(), item); err != nil {
			return item, err
		}
	}
	return item, nil
}

func persistEtag(assocFilename string, item stow.Item) error {
	eTag, err := item.ETag()
	if err != nil {
		return fmt.Errorf("Could not get eTag from remote object: %w", err)
	} else if eTag == "" {
		return nil
	}

	if err = os.WriteFile(eTagFileName(assocFilename), []byte(eTag), 0666); err != nil {
		return fmt.Errorf("Could not write eTag metadata file for cache persistence: %w", err)
	}
	return nil
}

func eTagFileName(assocFilename string) string {
	const eTagFileSuffix = ".etag"
	return assocFilename + eTagFileSuffix
}

func (sp *storeParams) claimCapacity() error {
	if sp.quotaSema != nil && !sp.quotaSema.P() {
		return errCapacityClaim
	}
	return nil
}

func (sp *storeParams) releaseCapacity() {
	const errCapacityReleaseMsg = "Could not release capacity, likely BUG"

	if sp.quotaSema != nil && !sp.quotaSema.V() {
		panic(errCapacityReleaseMsg)
	}
}

func sparseCopy(file *os.File, src io.Reader) (n int64, err error) {
	const minSeqSparseZeros = 4096
	in := bufio.NewReader(src)
	out := bufio.NewWriter(file)
	pendZeros := int64(0)

	var cur byte
	for {
		if cur, err = in.ReadByte(); err != nil {
			if err == io.EOF {
				if err = out.Flush(); err != nil {
					break
				}
				if pendZeros >= 0 {
					if penZeroMinusOne := pendZeros - 1; penZeroMinusOne > 0 {
						if _, err = file.Seek(penZeroMinusOne, os.SEEK_CUR); err != nil {
							break
						}
					}
					_, err = file.Write([]byte{0})
					n += pendZeros
				}
			}
			break
		}

		if cur == 0 {
			pendZeros++
		} else {
			if pendZeros > 0 {
				if pendZeros >= minSeqSparseZeros {
					if err = out.Flush(); err != nil {
						break
					}
					if _, err = file.Seek(pendZeros, os.SEEK_CUR); err != nil {
						break
					}
					n += pendZeros
				} else {
					for i := int64(0); i < pendZeros; i++ {
						if err = out.WriteByte(0); err != nil {
							break
						}
						n++
					}
					if err != nil {
						break
					}
				}
				pendZeros = 0
			}
			if err = out.WriteByte(cur); err != nil {
				break
			}
			n++
		}
	}
	if err != nil {
		return n, err
	}
	return n, out.Flush()
}
