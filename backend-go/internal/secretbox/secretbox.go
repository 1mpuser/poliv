// Шифрование секретов учётки (токен Яндекса) ключом, выведенным из JWT_SECRET.
// Совместимо с app/services/secret_box.py: та же деривация ключа (HKDF-SHA256, info "poliv:yandex-token")
// и тот же Fernet (AES-128-CBC + HMAC-SHA256), что использует cryptography.fernet.
package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"time"

	xhkdf "golang.org/x/crypto/hkdf"
)

var info = []byte("poliv:yandex-token")

// deriveKey — HKDF-SHA256(length=32, salt=nil, info="poliv:yandex-token") из JWT_SECRET.
func deriveKey(secret string) ([]byte, error) {
	r := xhkdf.New(sha256.New, []byte(secret), nil, info)
	key := make([]byte, 32)
	if _, err := io.ReadFull(r, key); err != nil {
		return nil, err
	}
	return key, nil
}

// Encrypt — зашифровать секрет (то, что положит Python-бэкенд, расшифрует).
func Encrypt(plain, secret string) (string, error) {
	key, err := deriveKey(secret)
	if err != nil {
		return "", err
	}
	signingKey, encKey := key[:16], key[16:]

	iv := make([]byte, aes.BlockSize)
	if _, err := rand.Read(iv); err != nil {
		return "", err
	}
	block, err := aes.NewCipher(encKey)
	if err != nil {
		return "", err
	}
	padded := pkcs7Pad([]byte(plain), aes.BlockSize)
	ct := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(ct, padded)

	ts := uint64(time.Now().Unix())
	// basic_parts в cryptography.fernet: b"\x80" + ts + iv + ct; HMAC считается по нему целиком.
	basic := []byte{0x80}
	basic = appendBasic(basic, ts, iv, ct)

	mac := hmac.New(sha256.New, signingKey)
	mac.Write(basic)
	sig := mac.Sum(nil)

	token := append([]byte{}, basic...)
	token = append(token, sig...)
	return base64.URLEncoding.EncodeToString(token), nil
}

// Decrypt — расшифровать токен (в т.ч. записанный Python-бэкендом). Неверный ключ/мусор → error.
func Decrypt(token, secret string) (string, error) {
	key, err := deriveKey(secret)
	if err != nil {
		return "", err
	}
	raw, err := base64.URLEncoding.DecodeString(token)
	if err != nil {
		return "", errors.New("base64 decode failed")
	}
	if len(raw) < 1+8+aes.BlockSize+32 || raw[0] != 0x80 {
		return "", errors.New("bad token")
	}
	signingKey, encKey := key[:16], key[16:]

	ts := raw[1:9]
	iv := raw[9 : 9+aes.BlockSize]
	ct := raw[9+aes.BlockSize : len(raw)-32]
	sig := raw[len(raw)-32:]

	// HMAC считается по \x80 + ts + iv + ct (включая байт версии)
	basic := raw[:len(raw)-32]
	mac := hmac.New(sha256.New, signingKey)
	mac.Write(basic)
	if !hmac.Equal(mac.Sum(nil), sig) {
		return "", errors.New("bad signature")
	}

	block, err := aes.NewCipher(encKey)
	if err != nil {
		return "", err
	}
	padded := make([]byte, len(ct))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(padded, ct)
	plain, err := pkcs7Unpad(padded, aes.BlockSize)
	if err != nil {
		return "", err
	}
	_ = ts
	return string(plain), nil
}

func appendBasic(dst []byte, ts uint64, iv, ct []byte) []byte {
	for i := 7; i >= 0; i-- {
		dst = append(dst, byte(ts>>(uint(i)*8)))
	}
	dst = append(dst, iv...)
	dst = append(dst, ct...)
	return dst
}

func pkcs7Pad(data []byte, blockSize int) []byte {
	pad := blockSize - len(data)%blockSize
	out := make([]byte, len(data)+pad)
	copy(out, data)
	for i := len(data); i < len(out); i++ {
		out[i] = byte(pad)
	}
	return out
}

func pkcs7Unpad(data []byte, blockSize int) ([]byte, error) {
	if len(data) == 0 || len(data)%blockSize != 0 {
		return nil, errors.New("bad padding length")
	}
	pad := int(data[len(data)-1])
	if pad == 0 || pad > blockSize || pad > len(data) {
		return nil, errors.New("bad padding")
	}
	for _, b := range data[len(data)-pad:] {
		if int(b) != pad {
			return nil, errors.New("bad padding")
		}
	}
	return data[:len(data)-pad], nil
}
