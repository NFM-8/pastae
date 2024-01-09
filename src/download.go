package main

import (
	"errors"
	"log"
	"net/http"
	"os"

	"github.com/julienschmidt/httprouter"
)

func servePaste(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	id := p.ByName("id")
	PASTAEMUTEX.RLock()
	data, ok := PASTAEMAP[id]
	PASTAEMUTEX.RUnlock()
	if ok {
		resp, error := fetchPaste(data)
		if error != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("content-type", data.ContentType)
		w.Write(resp)
		zeroByteArray(resp, len(resp))
	} else {
		http.NotFound(w, r)
	}
}

func servePasteS(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	id := p.ByName("id")
	PASTAEMUTEX.RLock()
	data, ok := PASTAEMAP[id]
	PASTAEMUTEX.RUnlock()
	if ok {
		resp, err := fetchPaste(data)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("content-type", data.ContentType)
		w.Write(resp)
		zeroByteArray(resp, len(resp))
	} else {
		var fname string
		var key []byte
		var nonce []byte
		var uid int64
		var contentType string
		var ukek []byte
		const qs = "SELECT fname,key,nonce,uid,ct,kek FROM data,users WHERE pid=$1 AND users.id=data.uid"
		err := DB.QueryRow(qs, id).Scan(&fname, &key, &nonce, &uid, &contentType, &ukek)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		var file []byte
		file, err = os.ReadFile(CONFIGURATION.DataPath + fname)
		if err != nil {
			log.Println(err)
			http.NotFound(w, r)
			return
		}
		sum := kdf(key, ukek)
		file, err = decrypt(file, sum[0:16], nonce)
		zeroByteArray(sum, 32)
		if err != nil {
			log.Println(err)
			http.NotFound(w, r)
			return
		}
		w.Header().Set("content-type", contentType)
		w.Write(file)
		zeroByteArray(file, len(file))
	}
}

func fetchPaste(pasta Pastae) ([]byte, error) {
	if PASTAELIST == nil {
		return []byte(""), errors.New("PASTAELIST is nil")
	}
	resp, err := decryptPaste(pasta)
	if err != nil {
		return []byte("ERROR"), err
	}
	if pasta.BurnAfterReading {
		PASTAEMUTEX.Lock()
		defer PASTAEMUTEX.Unlock()
		delete(PASTAEMAP, PASTAELIST.Front().Value.(Pastae).Id)
		PASTAELIST.Remove(PASTAELIST.Front())
	}
	return resp, nil
}

func decryptPaste(paste Pastae) ([]byte, error) {
	sum := kdf(paste.Key, KEK)
	data, err := decrypt(paste.Payload, sum[0:16], paste.Nonce)
	if err != nil {
		return []byte(""), err
	}
	zeroByteArray(sum, 32)
	return data, nil
}
