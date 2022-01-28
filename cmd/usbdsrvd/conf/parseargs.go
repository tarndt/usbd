package conf

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/tarndt/usbd/pkg/devices/objstore"
	"github.com/tarndt/usbd/pkg/devices/objstore/compress"
	"github.com/tarndt/usbd/pkg/devices/objstore/encrypt"
	"github.com/tarndt/usbd/pkg/usbdlib"

	"github.com/dustin/go-humanize"
	"github.com/graymeta/stow"
	"github.com/graymeta/stow/s3"
)

const (
	defKeyFile    = "key.aes"
	oneGiB        = 1024 * 1024 * 1024
	defStoreSize  = oneGiB
	defObjectSize = 64 * 1024 * 1024
)

//MustGetConfig successful reads configuration from command-line arguments and
// creates a Config or it exits with feedback for the invoking user
func MustGetConfig() *Config {

	var devKind string
	var help bool
	cfg := new(Config)

	//General options
	flag.StringVar(&devKind, "dev-type", "mem", "Type of device to back block device with: 'mem', 'file', 'dedup', 'objstore'.")
	flag.UintVar(&cfg.NBDDevCount, "nbd-max-devs", usbdlib.DefMaxNBDDevices, "If the NBD kernel module is loaded by this deamon how many NBD devices should it create")
	flag.StringVar(&cfg.StorageDirectory, "store-dir", "./", "Location to create new backing disk files in")
	flag.StringVar(&cfg.StorageName, "store-name", "test-lun", "File base name to use for new backing disk files")
	flagCapacityVar(&cfg.StorageBytes, "store-size", defStoreSize, "Amount of storage capcity to use for new backing files (ex. 100 MiB, 20 GiB)")
	flag.BoolVar(&help, "help", false, "Display help and exit")

	//Device type specific options

	//Dedup
	var IDStoreMemoryCache string
	flag.StringVar(&IDStoreMemoryCache, "dedup-memcache", "512 MiB", "Amount of memory to the dedup store ID cache (ex. 100 MiB, 20 GiB)")

	//Objectstore
	var objStoreConfigJSON, AESMode, AESKey, Compress string
	recAESMode := encrypt.ModeFromName(encrypt.ModeAESRecName).AlgoName()
	flagCapacityVar(&cfg.ObjStoreConfig.LocalDiskCacheBytes, "objstore-diskcache", 0, "Amount of disk for caching remote objects (0 implies fullbacking or ex. 100 MiB, 20 GiB)")
	flag.StringVar(&cfg.ObjStoreConfig.Kind, "objstore-kind", s3.Kind, "Type of remote objectstore: 's3', 'b2', 'local', 'azure', 'swift', 'google', 'oracle' or 'sftp'")
	flagCapacityVar(&cfg.ObjStoreConfig.ObjectBytes, "objstore-objsize", defObjectSize, "Size of remote objects (ex. 32 MiB, 1 GiB)")
	flag.StringVar(&objStoreConfigJSON, "objstore-cfg", mustGetDefObjStoreParams(), "JSON configuration (default assumes local minio [kind \"s3\"] with default settings)")
	flag.StringVar(&AESMode, "objstore-aesmode", recAESMode,
		fmt.Sprintf("AES encryption mode to use to encrypt remote objects: %q, %q, %q or %q for no encryption. %q is recommended.",
			encrypt.ModeAESCFBName, encrypt.ModeAESCTRName, encrypt.ModeAESOFBName, encrypt.ModeIdentityName, recAESMode),
	)
	flag.StringVar(&AESKey, "objstore-aeskey", "", "If AES is enabled; AES key to use to encrypt remote objects (if absent a key is generated and saved to ./"+defKeyFile+", otherwise use: key:<value>, file:<path>, env:<varname>")
	flag.StringVar(&Compress, "objstore-compress", compress.ModeS2Name,
		fmt.Sprintf("Compression algorithm to use for remote objects: %q, %q or %q for no compression)",
			compress.ModeS2Name, compress.ModeGzipName, compress.ModeIdentityName),
	)
	flag.UintVar(&cfg.ObjStoreConfig.ConcurFlush, "objstore-concurflush", 0, "Maximum number of dirty local objects to concurrently upload to the remote objectstore (0 implies use heuristic)")
	flag.DurationVar(&cfg.ObjStoreConfig.FlushInterval, "objstore-flushevery", time.Second*10, "Frequency in which dirty local objects are uploaded to the remote objectstore (0 disables autoflush)")

	//Process args set
	flag.Parse()

	if help {
		fmt.Printf("Usage: %s [optional: options see below...] [optional: NBD device to use ex. /dev/nbd0, if absent the first free device is used.]\n"+
			"Arguments starting with <driver name>-X are only applicable if dev-type=X is being set.\n"+
			"\tExample:\n"+"\t\t1 GiB device backed with by memory: ./usbdsrvd\n"+
			"\t\t8 GiB device backed by a file and exported specfically on /dev/nbd5: ./usbdsrvd -dev-type=file -store-dir=/tmp -store-name=testfilevol -store-size=8GiB /dev/nbd5\n"+
			"\t\t12 GiB device backed by file deduplicated using PebbleDB: ./usbdsrvd -dev-type=file -store-dir=/tmp -store-name=testdedupvol -store-size=12GiB\n"+
			"\t\t20 GiB device backed by a locally running S3/minio objectstore: ./usbdsrvd -dev-type=objstore -store-dir=/tmp -store-name=testobjvol -store-size=20GiB\n\n", os.Args[0])
		flag.PrintDefaults()
		os.Exit(0)
	}

	cfg.NBDDevName = flag.Arg(0)

	if cfg.BackingMode = NewBackingDevice(devKind); cfg.BackingMode == DevUnknown {
		log.Fatalf("Bad argument: Unknown backing device type of: %q", devKind)
	}

	if cfg.StorageName == "" {
		log.Fatalf("No volume name was provided (use -store-name=X)")
	}

	if cfg.BackingMode != DevMem {
		if cfg.StorageDirectory == "" {
			log.Fatalf("No storage directory was provided (use -store-dir=X)")
		}
		var err error
		if cfg.StorageDirectory, err = filepath.Abs(cfg.StorageDirectory); err != nil {
			log.Fatalf("Could not resolve storage directory (-store-dir=%q) to an absolute path: %s", cfg.StorageDirectory, err)
		} else if fstat, err := os.Stat(cfg.StorageDirectory); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				log.Fatalf("Provided storage directory (-store-dir=%q) does not exist", cfg.StorageDirectory)
			}
			log.Fatalf("Provided storage directory (-store-dir=%q) could not be be accessed", cfg.StorageDirectory)
		} else if !fstat.IsDir() {
			log.Fatalf("Provided storage directory (-store-dir=%q) is not a directory", cfg.StorageDirectory)
		}
	}

	switch cfg.BackingMode {
	case DevDedupFile:
		if IDStoreMemoryCache == "" {
			log.Fatalf("No memory quota for the dedup store ID cache was provided (use -dedup-memcache=X)")
		} else if bytes, err := humanize.ParseBytes(IDStoreMemoryCache); err != nil {
			log.Fatalf("Could not parse provided capacity for dedup store ID cache: %q: %s", IDStoreMemoryCache, err)
		} else {
			cfg.DedupConfig.IDStoreMemoryCacheBytes = int64(bytes)
		}

	case DevObjStore:
		mustGetObjStoreConfig(cfg, objStoreConfigJSON, AESMode, AESKey, Compress)
	}
	return cfg
}

func mustGetObjStoreConfig(cfg *Config, objStoreConfigJSON, AESMode, AESKey, Compress string) {
	switch strings.ToLower(cfg.ObjStoreConfig.Kind) {
	case "s3", "b2", "local", "azure", "swift", "google", "oracle", "sftp":
	case "":
		log.Fatalf("An objectstore kind must be provided (-objstore-kind=X)")
	default:
		log.Fatalf("Unknown objectstore kind was provided: %q", cfg.ObjStoreConfig.Kind)
	}

	if objStoreConfigJSON == "" {
		log.Fatalf("No JSON configuration was provided for remote objectstore (use -objstore-cfg=JSON)")
	}
	cfg.ObjStoreConfig.Config = make(stow.ConfigMap)
	if err := json.Unmarshal([]byte(objStoreConfigJSON), &cfg.ObjStoreConfig.Config); err != nil {
		log.Fatalf("Provided JSON configuration for remote objectstore could not be parsed: %s", err)
	}
	if err := objstore.ValidateConfig(cfg.ObjStoreConfig.Kind, cfg.ObjStoreConfig.Config); err != nil {
		log.Fatalf("Provided configuration for remote objectstore was not valid: %s", err)
	}

	if AESMode != encrypt.ModeIdentityName {
		if cfg.ObjStoreConfig.AESMode = encrypt.ModeFromName(AESMode); cfg.ObjStoreConfig.AESMode == encrypt.ModeUnknown {
			log.Fatalf("Unknown AES mode %q was provided", AESMode)
		}

		var err error
		if AESKey == "" {
			keyFile := filepath.Join(cfg.StorageDirectory, defKeyFile)
			if cfg.ObjStoreConfig.AESKey, err = os.ReadFile(keyFile); err != nil {
				if !errors.Is(err, os.ErrNotExist) {
					log.Fatalf("Could not read AES key file %q: %s", keyFile, err)
				}
				if cfg.ObjStoreConfig.AESKey, err = encrypt.MakeRandomAESKey(); err != nil {
					log.Fatalf("Could not create AES key: %s", err)
				}
				if err = os.WriteFile(keyFile, cfg.ObjStoreConfig.AESKey, 0666); err != nil {
					log.Fatalf("Could not write AES key to file: %q: %s", keyFile, err)
				}
				log.Printf("Generated AES-256 key and stored it in %q", keyFile)
			}
		} else {
			switch {
			case strings.HasPrefix(AESKey, "file:"):
				AESKey = strings.TrimPrefix(AESKey, "file:")
				if cfg.ObjStoreConfig.AESKey, err = os.ReadFile(AESKey); err != nil {
					log.Fatalf("Could not read AES key file %q: %s", AESKey, err)
				}

			case strings.HasPrefix(AESKey, "key:"):
				cfg.ObjStoreConfig.AESKey = []byte(strings.TrimPrefix(AESKey, "key:"))

			case strings.HasPrefix(AESKey, "env:"):
				AESKey = strings.TrimPrefix(AESKey, "env:")
				if envVal, exists := os.LookupEnv(AESKey); !exists {
					log.Fatalf("Could not read AES key from non-existent environment variable %q", AESKey)
				} else {
					cfg.ObjStoreConfig.AESKey = []byte(envVal)
				}

			default:
				log.Fatalf("AES key source %q is not valid", AESKey)
			}
			if err = encrypt.ValidAESKey(cfg.ObjStoreConfig.AESKey); err != nil {
				log.Fatalf("Could not validate provided AES key: %s", err)
			}
		}
	}

	if cfg.ObjStoreConfig.CompressMode = compress.ModeFromName(Compress); cfg.ObjStoreConfig.CompressMode == compress.ModeUnknown {
		log.Fatalf("Unknown compression mode %q was provided", Compress)
	}

	if cfg.ObjStoreConfig.ConcurFlush == 0 {
		cfg.ObjStoreConfig.ConcurFlush = recConcurFlush(int64(cfg.ObjStoreConfig.ObjectBytes))
	}
}

func mustGetDefObjStoreParams() string {
	cfg := stow.ConfigMap{
		s3.ConfigEndpoint:    "http://127.0.0.1:9000",
		s3.ConfigAccessKeyID: "minioadmin",
		s3.ConfigSecretKey:   "minioadmin",
	}

	JSON, err := json.Marshal(cfg)
	if err != nil {
		log.Fatalf("Could not marshal default object store JSON config: %s", err)
	}

	return string(JSON)
}

func recConcurFlush(objsize int64) uint {
	const budgetBytes = 1024 * 1024 * 1024 //1 GiB
	rec := runtime.NumCPU()

	if int64(rec)*objsize > budgetBytes {
		rec = int(budgetBytes / objsize)
	}
	if rec < 1 {
		rec = 1
	}
	return uint(rec)
}
