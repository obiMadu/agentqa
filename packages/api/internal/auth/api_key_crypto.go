package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

const apiKeyEncryptionKeySize = 32

func ParseAPIKeyEncryptionKey(value string) ([]byte, error) {
	if value == "" {
		return nil, errors.New("API_KEY_ENCRYPTION_KEY is required")
	}

	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("API_KEY_ENCRYPTION_KEY must be base64: %w", err)
	}
	if len(decoded) != apiKeyEncryptionKeySize {
		return nil, fmt.Errorf(
			"API_KEY_ENCRYPTION_KEY must decode to %d bytes, got %d",
			apiKeyEncryptionKeySize,
			len(decoded),
		)
	}

	return decoded, nil
}

func EncryptAPIKey(plain string, key []byte) (ciphertext string, nonce string, err error) {
	if len(key) != apiKeyEncryptionKeySize {
		return "", "", fmt.Errorf("encryption key must be %d bytes", apiKeyEncryptionKeySize)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", "", err
	}

	nonceBytes := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonceBytes); err != nil {
		return "", "", fmt.Errorf("nonce: %w", err)
	}

	ciphertextBytes := gcm.Seal(nil, nonceBytes, []byte(plain), nil)
	return base64.StdEncoding.EncodeToString(ciphertextBytes), base64.StdEncoding.EncodeToString(nonceBytes), nil
}

func DecryptAPIKey(ciphertext string, nonce string, key []byte) (string, error) {
	if len(key) != apiKeyEncryptionKeySize {
		return "", fmt.Errorf("encryption key must be %d bytes", apiKeyEncryptionKeySize)
	}

	ciphertextBytes, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("ciphertext decode: %w", err)
	}

	nonceBytes, err := base64.StdEncoding.DecodeString(nonce)
	if err != nil {
		return "", fmt.Errorf("nonce decode: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	if len(nonceBytes) != gcm.NonceSize() {
		return "", fmt.Errorf("nonce size mismatch")
	}

	plainBytes, err := gcm.Open(nil, nonceBytes, ciphertextBytes, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}

	return string(plainBytes), nil
}
