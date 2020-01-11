package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"io"
)

func encrypt(data []byte, key []byte, nonce []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return []byte("err"), errors.New("Error in encryption")
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return []byte("err"), errors.New("Error in encryption")
	}

	data = aesgcm.Seal(nil, nonce, []byte(data), nil)
	return data, nil
}

func decrypt(data []byte, key []byte, nonce []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return []byte("err"), errors.New("Error in decryption")
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return []byte("err"), errors.New("Error in decryption")
	}

	data, err = aesgcm.Open(nil, nonce, data, nil)
	if err != nil {
		return []byte("err"), errors.New("Error in decryption")
	}
	return data, nil
}

func generateRandomBytes(num int) ([]byte, error) {
	bytes := make([]byte, num)
	if _, err := io.ReadFull(rand.Reader, bytes); err != nil {
		return []byte("err"), errors.New("Error in random generation")
	}
	return bytes, nil
}
