package testutil

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"testing"
	"time"

	"github.com/tarndt/usbd/pkg/usbdlib"
	"github.com/tarndt/usbd/pkg/util/strms"
)

//CreateContext creates a context aware of the provided a testing.T's deadline
func CreateContext(t *testing.T) context.Context {
	const defaultTO = time.Minute
	deadline, hasDeadline := t.Deadline()
	if !hasDeadline {
		deadline = time.Now().Add(defaultTO)
	}

	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	t.Cleanup(cancel)
	return ctx
}

//TestDevSize verfies the provided device reports the correct size
func TestDevSize(t *testing.T, dev usbdlib.Device, expectedSize uint) {
	t.Run("dev-size", func(t *testing.T) {
		if actual := dev.Size(); actual != int64(expectedSize) {
			t.Fatalf("Expected device size to be %d but it was %d", expectedSize, actual)
		}
	})
}

//TestReadEmpty verifies the provided device reads as empty
func TestReadEmpty(t *testing.T, dev usbdlib.Device, boundaryBytes uint) {
	count := uint(dev.Size())

	t.Run("read-empty", func(t *testing.T) {
		t.Run("unbuffered", func(t *testing.T) {
			count := unbufCount(count)
			buf, rdr := make([]byte, 1), strms.NewReadAtReader(dev)
			for i := uint(0); i < count; i++ {
				if _, err := rdr.Read(buf); err != nil {
					t.Fatalf("Failed to read byte %d of %d: %s", i+1, count, err)
				}
				if buf[0] != 0 {
					t.Fatalf("Non-zero value %d found", buf[0])
				}
			}
		})

		t.Run("buffered", func(t *testing.T) {
			rdr := bufio.NewReader(strms.NewReadAtReader(dev))
			for i := uint(0); i < count; i++ {
				actual, err := rdr.ReadByte()
				if err != nil {
					t.Fatalf("Failed to read zero byte: %s", err)
				}
				if actual != 0 {
					t.Fatalf("Wrong value %d found, expecting %d", actual, 0)
				}
			}
		})

		t.Run("misaligned", func(t *testing.T) {
			if boundaryBytes < 1 {
				t.Skipf("No alignment boundary for test")
			}
			if count < boundaryBytes*2 {
				t.Skipf("Two segments worth of total capacity required for test")
			}

			t.Run("two-segments", func(t *testing.T) {
				twoSegSize := boundaryBytes * 2
				buf := make([]byte, twoSegSize)
				n, err := dev.ReadAt(buf, 0)
				switch {
				case err != nil:
					t.Fatalf("Failed two read two segments: %s", err)
				case uint(n) != twoSegSize:
					t.Fatalf("Read returned %d bytes rather than two segments (%d bytes)", n, twoSegSize)
				}
				for _, byte := range buf {
					if byte != 0 {
						t.Fatalf("Expected all zeros, found %d", byte)
					}
				}
			})

			t.Run("span-segments", func(t *testing.T) {
				buf := make([]byte, boundaryBytes)
				n, err := dev.ReadAt(buf, int64(boundaryBytes/2))
				switch {
				case err != nil:
					t.Fatalf("Failed two read spaning two segments: %s", err)
				case uint(n) != boundaryBytes:
					t.Fatalf("Read returned %d bytes rather than two segments (%d bytes)", n, boundaryBytes)
				}
				for _, byte := range buf {
					if byte != 0 {
						t.Fatalf("Expected all zeros, found %d", byte)
					}
				}
			})
		})
	})
}

//TestWriteReadPattern writes and reads a pattern to the provided device
func TestWriteReadPattern(t *testing.T, dev usbdlib.Device) (writtenHash []byte) {
	count := uint(dev.Size())

	t.Run("write-read-pattern", func(t *testing.T) {
		t.Run("unbuffered", func(t *testing.T) {
			count := unbufCount(count)
			buf, wtr := make([]byte, 1), strms.NewWriteAtWriter(dev)
			for i := uint(0); i < count; i++ {
				writeVal := byte(i % 256)
				if _, err := wtr.Write([]byte{writeVal}); err != nil {
					t.Fatalf("Failed to write byte %d of %d: %s", i+1, count, err)
				}
				if _, err := dev.ReadAt(buf, int64(i)); err != nil {
					t.Fatalf("Failed to read byte just written: %s", err)
				}
				if buf[0] != writeVal {
					t.Fatalf("Wrong value %d found at pos %d, expecting %d", buf[0], i, writeVal)
				}
			}
			if err := dev.Flush(); err != nil {
				t.Fatalf("Failed to flush device: %s", err)
			}
		})

		t.Run("buffered", func(t *testing.T) {
			const offset = 16

			getVal := func(idx uint) byte {
				if (idx/4096)%2 == 0 {
					return byte((idx + offset) % 256)
				}
				return 0
			}

			t.Run("write", func(t *testing.T) {
				hashWtr := sha256.New()
				wtr := bufio.NewWriter(io.MultiWriter(strms.NewWriteAtWriter(dev), hashWtr))
				for i := uint(0); i < count; i++ {
					if err := wtr.WriteByte(getVal(i)); err != nil {
						t.Fatalf("Failed to write byte %d of %d: %s", i+1, count, err)
					}
				}

				t.Run("flush", func(t *testing.T) {
					t.Run("buffer", func(t *testing.T) {
						if err := wtr.Flush(); err != nil {
							t.Fatalf("Failed to flush buffered writer: %s", err)
						}
					})
					writtenHash = hashWtr.Sum(nil)

					t.Run("device", func(t *testing.T) {
						if err := dev.Flush(); err != nil {
							t.Fatalf("Failed to flush device: %s", err)
						}
					})
				})
			})

			t.Run("read", func(t *testing.T) {
				t.Run("read-bytes", func(t *testing.T) {
					rdr := bufio.NewReader(strms.NewReadAtReader(dev))
					for i := uint(0); true; i++ {
						actual, err := rdr.ReadByte()
						if err != nil {
							if err == io.EOF && i == count {
								break
							}
							t.Fatalf("Failed to read byte index %d of %d just written: %s", i, count, err)
						}
						if expected := getVal(i); actual != expected {
							t.Fatalf("Wrong value %d found, expecting %d", actual, expected)
						}
					}
				})

				TestReadHash(t, dev, writtenHash)
			})
		})
	})

	return writtenHash
}

//TestReadHash confirms the SHA of the provided device matches the expected SHA
func TestReadHash(t *testing.T, dev usbdlib.Device, expectedHash []byte) {
	t.Run("read-hash", func(t *testing.T) {
		hashWtr := sha256.New()
		if n, err := io.Copy(hashWtr, bufio.NewReader(strms.NewReadAtReader(dev))); err != nil {
			t.Fatalf("Failed to calculate SHA256 of device: %s", err)
		} else if devSize := dev.Size(); n != devSize {
			t.Fatalf("While calculating SHA256 of device %d bytes were found instead of %d", n, devSize)
		} else if readHash := hashWtr.Sum(nil); !bytes.Equal(expectedHash, readHash) {
			t.Fatalf("SHA256 written to device was %s but %s was expected", hex.EncodeToString(expectedHash), hex.EncodeToString(readHash))
		}
	})
}

//TestClose confirms the device close without error and subsequent operations fail as expected
func TestClose(t *testing.T, dev usbdlib.Device) {
	t.Run("close", func(t *testing.T) {
		if err := dev.Close(); err != nil {
			t.Fatalf("Failed to close device: %s", err)
		}

		buf := make([]byte, 1)
		if _, err := dev.ReadAt(buf, 0); err == nil {
			t.Fatal("Expected error during read on closed device")
		}
		if _, err := dev.WriteAt(buf, 0); err == nil {
			t.Fatal("Expected error during write on closed device")
		}
		if err := dev.Flush(); err == nil {
			t.Fatal("Expected error during flush on closed device")
		}
	})
}

func unbufCount(count uint) uint {
	const ThreeMB = 3 * 1024 * 1024
	unbufCount := count / 3
	if unbufCount > ThreeMB {
		unbufCount = ThreeMB
	}
	return unbufCount
}
