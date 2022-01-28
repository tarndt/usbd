package encrypt

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"

	"github.com/tarndt/usbd/pkg/util"
)

//Mode represents an encryption mode
type Mode uint8

//Enumerate available modes and their textual names
const (
	ModeIdentity Mode = iota
	ModeUnknown
	ModeAESCTR
	ModeAESCFB
	ModeAESOFB
	ModeAESRec = ModeAESCTR

	ModeIdentityName = "identity"
	ModeAESCTRName   = "aes-ctr"
	ModeAESCFBName   = "aes-cfb"
	ModeAESOFBName   = "aes-ofb"
	ModeAESRecName   = "aes-rec"
	ModeUknownName   = "unknown"
)

//ModeFromName constructs a Mode from a textual name
func ModeFromName(name string) Mode {
	switch name {
	case "", ModeIdentityName:
		return ModeIdentity
	case ModeAESCTRName, ModeAESRecName:
		return ModeAESCTR
	case ModeAESCFBName:
		return ModeAESCFB
	case ModeAESOFBName:
		return ModeAESOFB
	}
	return ModeUnknown
}

//AlgoName returns the textual name of a Mode
func (m Mode) AlgoName() string {
	switch m {
	case ModeIdentity:
		return ModeIdentityName
	case ModeAESCTR:
		return ModeAESCTRName
	case ModeAESCFB:
		return ModeAESCFBName
	case ModeAESOFB:
		return ModeAESOFBName
	}
	return ModeUknownName
}

//String is a synonym for AlgoName
func (m Mode) String() string {
	return m.AlgoName()
}

//NewReader constructs a reader wrapper that applies this mode's decryption
func (m Mode) NewReader(rdr io.Reader, key, initVect []byte) (io.Reader, error) {
	var cipherConstructor func(cipher.Block, []byte) cipher.Stream
	switch m {
	case ModeIdentity:
		return rdr, nil
	case ModeAESCTR:
		cipherConstructor = cipher.NewCTR
	case ModeAESCFB:
		cipherConstructor = cipher.NewCFBDecrypter
	case ModeAESOFB:
		cipherConstructor = cipher.NewOFB
	default:
		return nil, fmt.Errorf("Cannot create decryptor for unknown cipher")
	}

	aesEnc, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("Could not create AES decryptor: %w", err)
	}

	if len(initVect) != aes.BlockSize {
		return nil, fmt.Errorf("Provided AES initialization vector has: %d bytes, rather than the required: %d bytes", len(initVect), aes.BlockSize)
	}

	return cipher.StreamReader{
		S: cipherConstructor(aesEnc, initVect),
		R: rdr,
	}, nil
}

//NewWriter constructs a writer wrapper that applies this mode's encryption
func (m Mode) NewWriter(wtr io.WriteCloser, key []byte) (encryptor io.WriteCloser, initVect []byte, err error) {
	if m == ModeIdentity {
		return wtr, nil, nil
	}

	aesEnc, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, fmt.Errorf("Could not create AES encryptor: %w", err)
	}

	initVect = make([]byte, aes.BlockSize)
	if _, err = rand.Read(initVect); err != nil {
		return nil, nil, fmt.Errorf("Could not read entropy source to populate AES initialization vector: %w", err)
	}

	var cipherConstructor func(cipher.Block, []byte) cipher.Stream
	switch m {
	case ModeAESCTR:
		cipherConstructor = cipher.NewCTR
	case ModeAESCFB:
		cipherConstructor = cipher.NewCFBEncrypter
	case ModeAESOFB:
		cipherConstructor = cipher.NewOFB
	default:
		return nil, nil, fmt.Errorf("Cannot create encryptor for unknown cipher")
	}

	return cipher.StreamWriter{
		S: cipherConstructor(aesEnc, initVect),
		W: noopWtrCloser{wtr}, //Sheild our writer from being closed
	}, initVect, nil
}

type noopWtrCloser struct {
	io.Writer
}

func (noopWtrCloser) Close() error { return nil }

//MakeRandomAESKey generates AES-256 (32 byte) key securely by using a cryptographic entropy source
func MakeRandomAESKey() ([]byte, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("Could not read entropy source to populate key: %w", err)
	}
	return key, nil
}

//ValidAESKey confirms the provided key is the correct length for AES/128/224/256
// and is not all zeros
func ValidAESKey(key []byte) error {
	switch len(key) {
	case 16, 24, 32:
	case 0:
		return fmt.Errorf("AES key is empty")
	default:
		return fmt.Errorf("AES key is of an unexpected length: %d bytes (Use 16 bytes for AES-128, 24 bytes for AES-192, or 32 bytes AES-256 [recommended])", len(key))
	}

	if util.IsZeros(key) {
		return fmt.Errorf("Provided AES key is all zeros (likely mistake)")
	}
	return nil
}
