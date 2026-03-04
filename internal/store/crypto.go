package store

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	fieldEncodingVersion = "v1"
	fieldPrefix          = "ENC["
)

type kdfParams struct {
	Name    string `yaml:"name"`
	Salt    string `yaml:"salt"`
	Time    uint32 `yaml:"time"`
	Memory  uint32 `yaml:"memory"`
	Threads uint8  `yaml:"threads"`
	KeyLen  uint32 `yaml:"key_len"`
}

func defaultKDFParams(salt []byte) kdfParams {
	return kdfParams{
		Name:    "argon2id",
		Salt:    encodeB64(salt),
		Time:    3,
		Memory:  64 * 1024,
		Threads: 4,
		KeyLen:  32,
	}
}

func encryptStringField(value string, key []byte) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", nil
	}

	nonce, err := randomBytes(12)
	if err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}

	ciphertext, err := encryptWithKey([]byte(value), key, nonce)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf(
		"%s%s,%s,%s]",
		fieldPrefix,
		fieldEncodingVersion,
		encodeB64(nonce),
		encodeB64(ciphertext),
	), nil
}

func decryptStringField(value string, key []byte) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", nil
	}
	if !strings.HasPrefix(value, fieldPrefix) || !strings.HasSuffix(value, "]") {
		return "", errors.New("field is not encrypted")
	}

	inner := strings.TrimSuffix(strings.TrimPrefix(value, fieldPrefix), "]")
	parts := strings.Split(inner, ",")
	if len(parts) != 3 {
		return "", errors.New("invalid encrypted field format")
	}
	if parts[0] != fieldEncodingVersion {
		return "", fmt.Errorf("unsupported field encoding version: %s", parts[0])
	}

	nonce, err := decodeB64(parts[1])
	if err != nil {
		return "", fmt.Errorf("decode nonce: %w", err)
	}
	ciphertext, err := decodeB64(parts[2])
	if err != nil {
		return "", fmt.Errorf("decode ciphertext: %w", err)
	}

	plaintext, err := decryptWithKey(ciphertext, key, nonce)
	if err != nil {
		return "", err
	}
	defer zeroBytes(plaintext)

	return string(plaintext), nil
}

func encryptWithKey(plaintext, key, nonce []byte) ([]byte, error) {
	if len(key) == 0 {
		return nil, errors.New("empty encryption key")
	}
	if len(plaintext) == 0 {
		return nil, errors.New("empty plaintext")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("new aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("new gcm: %w", err)
	}

	return gcm.Seal(nil, nonce, plaintext, nil), nil
}

func decryptWithKey(ciphertext, key, nonce []byte) ([]byte, error) {
	if len(key) == 0 {
		return nil, errors.New("empty encryption key")
	}
	if len(ciphertext) == 0 {
		return nil, errors.New("empty ciphertext")
	}
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
