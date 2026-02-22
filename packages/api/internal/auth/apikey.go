package auth

import (
  "crypto/rand"
  "crypto/sha256"
  "encoding/base64"
  "encoding/hex"
  "fmt"
)

const APIKeyPrefix = "aq_"

func GenerateAPIKey() (plain string, hash string, prefix string, err error) {
  random := make([]byte, 32)
  if _, err = rand.Read(random); err != nil {
    return "", "", "", fmt.Errorf("rand: %w", err)
  }

  encoded := base64.RawURLEncoding.EncodeToString(random)
  plain = APIKeyPrefix + encoded
  hash = HashAPIKey(plain)

  if len(plain) >= 8 {
    prefix = plain[:8]
  } else {
    prefix = plain
  }

  return plain, hash, prefix, nil
}

func HashAPIKey(plain string) string {
  sum := sha256.Sum256([]byte(plain))
  return hex.EncodeToString(sum[:])
}
