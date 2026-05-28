package stream

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
)

const streamKeyPrefix = "live_"

func GenerateKey() (plain, hash string, err error) {
	var buf [32]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", "", fmt.Errorf("rand: %w", err)
	}
	plain = streamKeyPrefix + base64.RawURLEncoding.EncodeToString(buf[:])
	return plain, HashKey(plain), nil
}

func HashKey(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func VerifyKey(plain, hash string) bool {
	return subtle.ConstantTimeCompare([]byte(HashKey(plain)), []byte(hash)) == 1
}
