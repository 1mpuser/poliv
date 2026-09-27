package passwords

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"

	"golang.org/x/crypto/scrypt"
)

const (
	// Параметры scrypt: ~16 МБ памяти на проверку — терпимо для 1 vCPU / 2 ГБ (как в Python).
	N          = 1 << 14
	R          = 8
	P          = 1
	keyLen     = 64
	MinLength  = 8
	MaxLength  = 128
	SaltLength = 16
)

// Без похожих символов: 0/O, 1/l/I
const Alphabet = "abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// HashPassword — тот же формат, что app/passwords.py: scrypt$N$r$p$<b64 salt>$<b64 digest>.
func HashPassword(password string) (string, error) {
	salt := make([]byte, SaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	digest, err := scrypt.Key([]byte(password), salt, N, R, P, keyLen)
	if err != nil {
		return "", err
	}
	return "scrypt$" + strconv.Itoa(N) + "$" + strconv.Itoa(R) + "$" + strconv.Itoa(P) + "$" +
		base64.StdEncoding.EncodeToString(salt) + "$" + base64.StdEncoding.EncodeToString(digest), nil
}

// VerifyPassword — проверяет хэш (в т.ч. старый, от Python-бэкенда). Заглушки вроде "!" → false.
func VerifyPassword(password, stored string) bool {
	parts := strings.Split(stored, "$")
	if len(parts) != 6 || parts[0] != "scrypt" {
		return false
	}
	n, err := strconv.Atoi(parts[1])
	if err != nil {
		return false
	}
	r, err := strconv.Atoi(parts[2])
	if err != nil {
		return false
	}
	p, err := strconv.Atoi(parts[3])
	if err != nil {
		return false
	}
	salt, err := base64.StdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}
	expected, err := base64.StdEncoding.DecodeString(parts[5])
	if err != nil {
		return false
	}
	actual, err := scrypt.Key([]byte(password), salt, n, r, p, len(expected))
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(actual, expected) == 1
}

// GeneratePassword — пароль из алфавита без похожих символов, как generate_password в Python.
func GeneratePassword(length int) (string, error) {
	if length <= 0 {
		length = 16
	}
	out := make([]byte, length)
	for i := range out {
		b := make([]byte, 2)
		if _, err := rand.Read(b); err != nil {
			return "", err
		}
		out[i] = Alphabet[(int(b[0])<<8+int(b[1]))%len(Alphabet)]
	}
	return string(out), nil
}

func CheckPassword(password string) error {
	if len(password) < MinLength || len(password) > MaxLength {
		return errors.New("Пароль — от " + strconv.Itoa(MinLength) + " до " + strconv.Itoa(MaxLength) + " символов")
	}
	return nil
}
