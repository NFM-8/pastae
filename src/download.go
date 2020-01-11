package main

import (
	"crypto/sha512"
	"errors"
	"net/http"

	"github.com/julienschmidt/httprouter"
)

func servePaste(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	id := p.ByName("id")
	pastaeMutex.RLock()
	data, ok := pastaes[id]
	pastaeMutex.RUnlock()
	if ok {
		resp, error := fetchPaste(data)
		if error != nil {
			http.NotFound(w, r)
			return
		} else {
			w.Header().Set("content-type", data.ContentType)
			w.Write(resp)
			for i := 0; i < len(resp); i++ {
				resp[i] = 0
			}
		}
	} else {
		http.NotFound(w, r)
	}
}

func fetchPaste(pasta Pastae) ([]byte, error) {
	resp, error := decryptPaste(pasta)
	if error != nil {
		return []byte("ERROR"), errors.New("Error fetching paste")
	}
	if pasta.BurnAfterReading {
		pastaeMutex.Lock()
		if pasta.Next != nil {
			pasta.Next.Prev = pasta.Prev
		} else {
			lastPastae = pasta.Prev
		}
		if pasta.Prev != nil {
			pasta.Prev.Next = pasta.Next
		} else {
			firstPastae = pasta.Next
		}
		delete(pastaes, pasta.Id)
		pastaeMutex.Unlock()
	}
	return resp, nil
}

func decryptPaste(paste Pastae) ([]byte, error) {
	var key [32]byte
	for i := 0; i < 16; i++ {
		key[i] = paste.Key[i]
	}
	for i := 16; i < 32; i++ {
		key[i] = kek[i-16]
	}
	sum := sha512.Sum512(key[0:32])
	data, error := decrypt(paste.Payload, sum[0:16], paste.Nonce)
	if error != nil {
		return []byte(""), errors.New("Error in decryption")
	}
	for i := 0; i < 32; i++ {
		sum[i] = 0
	}
	return data, nil
}
