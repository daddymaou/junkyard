package crypto

import (
	"bytes"
	"testing"
)

func TestEncryptDecryptStreamRoundTrip(t *testing.T) {
	key := DeriveKey("A7B9X2", "enc")
	plain := make([]byte, StreamChunkSize*2+137)
	for i := range plain {
		plain[i] = byte((i*19 + 3) % 251)
	}

	var encrypted bytes.Buffer
	if err := EncryptStream(key, bytes.NewReader(plain), &encrypted); err != nil {
		t.Fatalf("encrypt stream: %v", err)
	}
	if bytes.Equal(encrypted.Bytes(), plain) {
		t.Fatal("encrypted stream matches plaintext")
	}

	var decrypted bytes.Buffer
	if err := DecryptStream(key, bytes.NewReader(encrypted.Bytes()), &decrypted); err != nil {
		t.Fatalf("decrypt stream: %v", err)
	}
	if !bytes.Equal(decrypted.Bytes(), plain) {
		t.Fatal("decrypted stream differs from plaintext")
	}
}

func TestDecryptStreamRejectsTampering(t *testing.T) {
	key := DeriveKey("A7B9X2", "enc")
	var encrypted bytes.Buffer
	if err := EncryptStream(key, bytes.NewBufferString("verify authenticated chunks"), &encrypted); err != nil {
		t.Fatal(err)
	}
	data := append([]byte(nil), encrypted.Bytes()...)
	data[len(data)-1] ^= 0xff
	if err := DecryptStream(key, bytes.NewReader(data), &bytes.Buffer{}); err == nil {
		t.Fatal("tampered ciphertext was accepted")
	}
}

func TestEncryptDecryptEmptyStream(t *testing.T) {
	key := DeriveKey("A7B9X2", "enc")
	var encrypted bytes.Buffer
	if err := EncryptStream(key, bytes.NewReader(nil), &encrypted); err != nil {
		t.Fatal(err)
	}
	var decrypted bytes.Buffer
	if err := DecryptStream(key, &encrypted, &decrypted); err != nil {
		t.Fatal(err)
	}
	if decrypted.Len() != 0 {
		t.Fatalf("decrypted empty stream has %d bytes", decrypted.Len())
	}
}
