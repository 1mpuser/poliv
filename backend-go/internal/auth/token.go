package auth

import (
	"errors"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// IssueToken — JWT HS256, совместимый с PyJWT (app/auth.py): sub=id, ver=token_version, exp.
// PyJWT кодирует заголовок и claims с sort_keys=True и separators (":",",") — то же делает golang-jwt.
func IssueToken(secret string, expireDays, userID, tokenVersion int) (string, error) {
	claims := jwt.MapClaims{
		"sub": strconv.Itoa(userID),
		"ver": tokenVersion,
		"exp": time.Now().Add(time.Duration(expireDays) * 24 * time.Hour).Unix(),
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
}

// ParseToken — разбирает токен, выданный и Python, и Go. Проверяет подпись и срок.
func ParseToken(secret, raw string) (userID, tokenVersion int, err error) {
	parsed, err := jwt.Parse(raw, func(t *jwt.Token) (interface{}, error) {
		return []byte(secret), nil
	}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired())
	if err != nil {
		return 0, 0, err
	}
	if !parsed.Valid {
		return 0, 0, errors.New("invalid token")
	}
	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return 0, 0, errors.New("bad claims")
	}
	sub, ok := claims["sub"].(string)
	if !ok {
		return 0, 0, errors.New("bad sub")
	}
	userID, err = strconv.Atoi(sub)
	if err != nil {
		return 0, 0, errors.New("bad sub")
	}
	ver, ok := claims["ver"].(float64)
	if !ok {
		return 0, 0, errors.New("bad ver")
	}
	return userID, int(ver), nil
}
