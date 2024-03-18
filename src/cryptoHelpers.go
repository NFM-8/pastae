package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha512"
	"io"
)

func encrypt(data []byte, key []byte, nonce []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return []byte(err.Error()), err
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return []byte(err.Error()), err
	}

	data = aesgcm.Seal(nil, nonce, []byte(data), nil)
	return data, nil
}

func decrypt(data []byte, key []byte, nonce []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return []byte(err.Error()), err
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return []byte(err.Error()), err
	}
	data, err = aesgcm.Open(nil, nonce, data, nil)
	if err != nil {
		return []byte(err.Error()), err
	}
	return data, nil
}

func generateRandomBytes(num int) ([]byte, error) {
	bytes := make([]byte, num)
	if _, err := io.ReadFull(rand.Reader, bytes); err != nil {
		return []byte(err.Error()), err
	}
	return bytes, nil
}

func kdf(key []byte, kek []byte) []byte {
	var ekey [32]byte
	for i := range ekey {
		if i < 16 {
			ekey[i] = key[i]
		} else {
			ekey[i] = kek[i-16]
		}
	}
	sum := sha512.Sum512(ekey[0:32])
	return sum[0:32]
}

func zeroByteArray(arr []byte) {
	for i := range arr {
		arr[i] = 0
	}
}
