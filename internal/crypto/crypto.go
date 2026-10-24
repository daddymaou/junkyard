package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"errors"
	"golang.org/x/crypto/pbkdf2"
	"hash"
	"io"
)

const (
	KeySize         = 32
	SaltSize        = 16
	NonceSize       = 12
	StreamChunkSize = 1024 * 1024
)

func DeriveKey(code, purpose string) []byte {
	return pbkdf2.Key([]byte(code), []byte("junkyard-"+purpose), 100000, KeySize, func() hash.Hash {
		return sha256.New()
	})
}

func Encrypt(key []byte, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, NonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

func Decrypt(key []byte, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(ciphertext) < NonceSize {
		return nil, errors.New("ciphertext too short")
	}
	nonce := ciphertext[:NonceSize]
	ciphertext = ciphertext[NonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}
	return plaintext, nil
}

func EncryptStream(key []byte, src io.Reader, dst io.Writer) error {
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}

	buf := make([]byte, StreamChunkSize)
	var index uint64
	for {
		n, readErr := io.ReadFull(src, buf)
		if readErr != nil && readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
			return readErr
		}
		if n == 0 {
			return nil
		}

		nonce := make([]byte, NonceSize)
		if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
			return err
		}
		var length [4]byte
		binary.BigEndian.PutUint32(length[:], uint32(n))
		sealed := gcm.Seal(nil, nonce, buf[:n], streamAAD(index, length[:]))
		if err := writeAll(dst, length[:]); err != nil {
			return err
		}
		if err := writeAll(dst, nonce); err != nil {
			return err
		}
		if err := writeAll(dst, sealed); err != nil {
			return err
		}
		index++
		if readErr == io.ErrUnexpectedEOF {
			return nil
		}
	}
}

func DecryptStream(key []byte, src io.Reader, dst io.Writer) error {
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}

	var index uint64
	var length [4]byte
	for {
		_, err := io.ReadFull(src, length[:])
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return errors.New("truncated encrypted stream length")
		}
		n := binary.BigEndian.Uint32(length[:])
		if n == 0 || n > StreamChunkSize {
			return errors.New("invalid encrypted stream chunk length")
		}

		nonce := make([]byte, NonceSize)
		if _, err := io.ReadFull(src, nonce); err != nil {
			return errors.New("truncated encrypted stream nonce")
		}
		sealed := make([]byte, int(n)+gcm.Overhead())
		if _, err := io.ReadFull(src, sealed); err != nil {
			return errors.New("truncated encrypted stream chunk")
		}
		plain, err := gcm.Open(nil, nonce, sealed, streamAAD(index, length[:]))
		if err != nil {
			return err
		}
		if err := writeAll(dst, plain); err != nil {
			return err
		}
		index++
	}
}

func streamAAD(index uint64, length []byte) []byte {
	aad := make([]byte, 12)
	binary.BigEndian.PutUint64(aad, index)
	copy(aad[8:], length)
	return aad
}

func writeAll(w io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := w.Write(data)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		data = data[n:]
	}
	return nil
}

func ConstantTimeCompare(a, b []byte) bool {
	return subtle.ConstantTimeCompare(a, b) == 1
}
