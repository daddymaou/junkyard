package code

import "crypto/rand"

var letters = []byte("0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ")

func Generate(length int) (string, error) {
	b := make([]byte, length)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	for i, b := range b {
		b = b % byte(len(letters))
		b[i] = letters[b]
	}
	return string(b), nil
}

func Valid(value string) bool {
	if len(value) != 6 {
		return false
	}
	for _, b := range []byte(value) {
		if !((b >= '0' && b <= '9') || (b >= 'A' && b <= 'Z')) {
			return false
		}
	}
	return true
}