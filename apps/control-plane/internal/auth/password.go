// Package auth คุม password hashing, JWT session, cookie และ login rate limit
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"math/big"

	"golang.org/x/crypto/bcrypt"
)

const bcryptCost = 12

const MinPasswordLength = 10

func HashPassword(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

const passwordAlphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

const generatedPasswordLength = 20

// GeneratePassword ใช้ crypto/rand เท่านั้น — initial password ของ user ใหม่/reset
func GeneratePassword() (string, error) {
	out := make([]byte, generatedPasswordLength)
	max := big.NewInt(int64(len(passwordAlphabet)))
	for i := range out {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		out[i] = passwordAlphabet[n.Int64()]
	}
	return string(out), nil
}

// GenerateSecretHex สร้าง opaque secret สำหรับ node token
func GenerateSecretHex(nBytes int) (string, error) {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// HashToken คืน SHA-256 hex ของ token ทั้งเส้น — ตรงกับคอลัมน์ nodes.agent_token_hash
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
