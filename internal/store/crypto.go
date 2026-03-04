package store

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	"golang.org/x/crypto/argon2"
)

const (
	currentVersion = 1
)

type kdfParams struct {
	Name    string `json:"name"`
	Salt    string `json:"salt"`
	Time    uint32 `json:"time"`
	Memory  uint32 `json:"memory"`
	Threads uint8  `json:"threads"`
	KeyLen  uint32 `json:"key_len"`
}

type cipherParams struct {
	Name       string `json:"name"`
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
}

type envelope struct {
	Version int          `json:"version"`
	KDF     kdfParams    `json:"kdf"`
	Cipher  cipherParams `json:"cipher"`
}

func encrypt(plaintext, passphrase []byte) ([]byte, error) {
	if len(passphrase) == 0 {
		return nil, errors.New("empty passphrase")
	}

	salt, err := randomBytes(16)
	if err != nil {
		return nil, fmt.Errorf("generate salt: %w", err)
	}
	nonce, err := randomBytes(12)
	if err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}

	params := kdfParams{
		Name:    "argon2id",
		Salt:    encodeB64(salt),
		Time:    3,
		Memory:  64 * 1024,
		Threads: 4,
		KeyLen:  32,
	}

	key := deriveKey(passphrase, salt, params.Time, params.Memory, params.Threads, params.KeyLen)
	defer zeroBytes(key)

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("new aes cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("new gcm: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)

	out := envelope{
		Version: currentVersion,
		KDF:     params,
		Cipher: cipherParams{
			Name:       "aes-256-gcm",
			Nonce:      encodeB64(nonce),
			Ciphertext: encodeB64(ciphertext),
		},
	}

	serialized, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal envelope: %w", err)
	}

	return serialized, nil
}

func decrypt(payload, passphrase []byte) ([]byte, error) {
	if len(passphrase) == 0 {
		return nil, errors.New("empty passphrase")
	}

	var doc envelope
	if err := json.Unmarshal(payload, &doc); err != nil {
		return nil, fmt.Errorf("decode envelope: %w", err)
	}

	if doc.Version != currentVersion {
		return nil, fmt.Errorf("unsupported config version: %d", doc.Version)
	}
	if doc.KDF.Name != "argon2id" {
		return nil, fmt.Errorf("unsupported kdf: %s", doc.KDF.Name)
	}
	if doc.Cipher.Name != "aes-256-gcm" {
		return nil, fmt.Errorf("unsupported cipher: %s", doc.Cipher.Name)
	}

	salt, err := decodeB64(doc.KDF.Salt)
	if err != nil {
		return nil, fmt.Errorf("decode salt: %w", err)
	}
	nonce, err := decodeB64(doc.Cipher.Nonce)
	if err != nil {
		return nil, fmt.Errorf("decode nonce: %w", err)
	}
	ciphertext, err := decodeB64(doc.Cipher.Ciphertext)
	if err != nil {
		return nil, fmt.Errorf("decode ciphertext: %w", err)
	}

	key := deriveKey(passphrase, salt, doc.KDF.Time, doc.KDF.Memory, doc.KDF.Threads, doc.KDF.KeyLen)
	defer zeroBytes(key)

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("new aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("new gcm: %w", err)
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, ErrInvalidPassword
	}

	return plaintext, nil
}

func deriveKey(passphrase, salt []byte, time, memory uint32, threads uint8, keyLen uint32) []byte {
	return argon2.IDKey(passphrase, salt, time, memory, threads, keyLen)
}

func randomBytes(size int) ([]byte, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return nil, err
	}
	return buf, nil
}

func encodeB64(input []byte) string {
	return base64.StdEncoding.EncodeToString(input)
}

func decodeB64(input string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(input)
}

func zeroBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
