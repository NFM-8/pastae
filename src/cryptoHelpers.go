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
		return []byte("err"), err
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return []byte("err"), err
	}

	data = aesgcm.Seal(nil, nonce, []byte(data), nil)
	return data, nil
}

func decrypt(data []byte, key []byte, nonce []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return []byte("err"), err
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return []byte("err"), err
	}
	data, err = aesgcm.Open(nil, nonce, data, nil)
	if err != nil {
		return []byte("err"), err
	}
	return data, nil
}

func generateRandomBytes(num int) ([]byte, error) {
	bytes := make([]byte, num)
	if _, err := io.ReadFull(rand.Reader, bytes); err != nil {
		return []byte("err"), err
	}
	return bytes, nil
}

func kdf(key []byte, kek []byte) []byte {
	var ekey [32]byte
	for i := 0; i < 16; i++ {
		ekey[i] = key[i]
	}
	for i := 16; i < 32; i++ {
		ekey[i] = kek[i-16]
	}
	sum := sha512.Sum512(ekey[0:32])
	return sum[0:32]
}

func zeroByteArray(arr []byte, len int) {
	for i := 0; i < len; i++ {
		arr[i] = 0
	}
}
