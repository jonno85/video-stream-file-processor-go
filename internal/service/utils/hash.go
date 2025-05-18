package utils

import (
	"crypto/sha256"
	"encoding/hex"
)

func ComputeHash(data []byte) (string, error) {
	hash := sha256.New()
	hash.Write(data)
	return hex.EncodeToString(hash.Sum(nil)), nil
}