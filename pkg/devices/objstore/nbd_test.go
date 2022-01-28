package objstore

import (
	"net/http/httptest"
	"os"
	"testing"

	"github.com/tarndt/usbd/pkg/devices/objstore/compress"
	"github.com/tarndt/usbd/pkg/devices/objstore/encrypt"
	"github.com/tarndt/usbd/pkg/devices/testutil"

	"github.com/graymeta/stow"
	"github.com/graymeta/stow/s3"
	"github.com/johannesboyne/gofakes3"
	"github.com/johannesboyne/gofakes3/backend/s3mem"
)

func TestNBD(t *testing.T) {
	const rootUID = 0
	if os.Geteuid() != rootUID {
		t.Skip("Must be root for this test, try: go test -c -race && sudo ./objstore.test -test.v -test.timeout=120s && rm ./objstore.test")
	}

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
		t.Fatalf("Could not create s3 object store: %s", err)
	}

	key, err := encrypt.MakeRandomAESKey()
	if err != nil {
		t.Fatalf("Could not create AES key: %s", err)
	}
	const totalBytes = 256 * 1024 * 1024 //256 MB
	dev := createDevice(
		t, createContainer(t, store), totalBytes, totalBytes,
		OptCompressRemoteObjects(compress.ModeS2),
		OptEncrypt{Mode: encrypt.ModeAESRec, Key: key},
	)

	testutil.TestNBD(t, dev, totalBytes)
}
