package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

const hashPrefix = "$argon2id$v=19$m=65536,t=3,p=2$"
const passwordRule = "Password minimal 8 karakter, dengan huruf besar, huruf kecil, angka dan simbol"

func validNewPassword(password string) bool {
	if !utf8.ValidString(password) || utf8.RuneCountInString(password) < 8 {
		return false
	}
	var upper, lower, number, symbol bool
	for _, character := range password {
		upper = upper || unicode.IsUpper(character)
		lower = lower || unicode.IsLower(character)
		number = number || (character >= '0' && character <= '9')
		symbol = symbol || unicode.IsPunct(character) || unicode.IsSymbol(character)
	}
	return upper && lower && number && symbol
}

func HashPassword(password string) (string, error) {
	if !validNewPassword(password) {
		return "", errors.New(passwordRule)
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, 3, 64*1024, 2, 32)
	return hashPrefix + base64.RawStdEncoding.EncodeToString(salt) + "$" + base64.RawStdEncoding.EncodeToString(key), nil
}

func VerifyPassword(password, encoded string) bool {
	if !strings.HasPrefix(encoded, hashPrefix) {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(encoded, hashPrefix), "$")
	if len(parts) != 2 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[0])
	if err != nil || len(salt) != 16 {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[1])
	if err != nil || len(want) != 32 {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, 3, 64*1024, 2, 32)
	return subtle.ConstantTimeCompare(got, want) == 1
}
