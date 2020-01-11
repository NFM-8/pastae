package main

import (
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"io"
	"io/ioutil"
	"net/http"

	"github.com/julienschmidt/httprouter"
)

func uploadPaste(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	err := r.ParseMultipartForm(configuration.MaxEntrySize + 4096)
	if err != nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	contentType := r.FormValue("content-type")
	bar := r.FormValue("bar") == "bar"
	if contentType == "text/plain" {
		id := insertPaste([]byte(r.FormValue("data")), bar, contentType)
		if err := r.Body.Close(); err != nil {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(id))
		return
	}
	if contentType == "image/jpeg" || contentType == "image/png" {
		file, header, err := r.FormFile("file")
		if err != nil || header.Size > configuration.MaxEntrySize {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		data, err := ioutil.ReadAll(io.LimitReader(file, configuration.MaxEntrySize))
		if err != nil {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		id := insertPaste(data, bar, contentType)
		if err := r.Body.Close(); err != nil {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(id))
		return
	}
	w.WriteHeader(http.StatusNoContent)
	return
}

func insertPaste(pasteData []byte, bar bool, contentType string) string {
	if len(pastaes) >= configuration.MaxEntries {
		if firstPastae != nil {
			id := firstPastae.Id
			pastaeMutex.Lock()
			if firstPastae.Next != nil {
				firstPastae.Next.Prev = nil
				firstPastae = firstPastae.Next
			} else {
				firstPastae = nil
				lastPastae = nil
			}
			delete(pastaes, id)
			pastaeMutex.Unlock()
		}
	}
	var paste Pastae
	paste.BurnAfterReading = bar
	nonce, error := generateRandomBytes(12)
	if error != nil {
		return "ERROR"
	}
	key, error := generateRandomBytes(16)
	if error != nil {
		return "ERROR"
	}
	paste.ContentType = contentType
	paste.Nonce = nonce
	paste.Key = key
	pasteData, error = encryptData(pasteData, key, nonce)
	if error != nil {
		return "ERROR"
	}
	paste.Payload = pasteData
	rnd, error := generateRandomBytes(12)
	if error != nil {
		return "ERROR"
	}
	id := hex.EncodeToString(rnd)
	paste.Next = nil
	paste.Prev = nil
	paste.Id = id
	pastaeMutex.Lock()
	pastaes[id] = paste
	if lastPastae != nil {
		lastPastae.Next = &paste
		paste.Prev = lastPastae
		lastPastae = &paste
	} else {
		firstPastae = &paste
		lastPastae = &paste
	}
	pastaeMutex.Unlock()
	return configuration.URL + id
}

func encryptData(payload []byte, key []byte, nonce []byte) ([]byte, error) {
	var ekey [32]byte
	for i := 0; i < 16; i++ {
		ekey[i] = key[i]
	}
	for i := 16; i < 32; i++ {
		ekey[i] = kek[i-16]
	}
	sum := sha512.Sum512(ekey[0:32])
	var error error
	payload, error = encrypt(payload, sum[0:16], nonce)
	if error != nil {
		for i := 0; i < 32; i++ {
			sum[i] = 0
		}
		return nil, errors.New("ERROR")
	}
	for i := 0; i < 32; i++ {
		sum[i] = 0
	}
	return payload, nil
}
