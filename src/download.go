package main

import (
	"errors"
	"log"
	"net/http"
	"os"

	"github.com/julienschmidt/httprouter"
)

func servePaste(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	if r == nil {
		log.Println("http.Request is nil")
		return
	}
	PASTAEMUTEX.RLock()
	data, ok := PASTAEMAP[p.ByName("id")]
	PASTAEMUTEX.RUnlock()
	if ok {
		resp, err := fetchPaste(data)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("content-type", data.ContentType)
		_, err = w.Write(resp)
		if err != nil {
			log.Println(err.Error())
		}
		zeroByteArray(resp)
	} else {
		http.NotFound(w, r)
	}
}

func servePasteS(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	if r == nil {
		log.Println("http.Request is nil")
		return
	}
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
		_, err = w.Write(resp)
		if err != nil {
			log.Println(err.Error())
		}
		zeroByteArray(resp)
	} else {
		var fname string
		var key []byte
		var nonce []byte
		var uid int64
		var contentType string
		var ukek []byte
		const qs string = "SELECT fname,key,nonce,uid,ct,kek FROM data,users WHERE pid=$1 AND users.id=data.uid"
		err := DB.QueryRow(qs, id).Scan(&fname, &key, &nonce, &uid, &contentType, &ukek)
		if err != nil {
			log.Println(err)
			http.NotFound(w, r)
			return
		}
		file, err := os.ReadFile(CONFIGURATION.DataPath + fname)
		if err != nil {
			log.Println(err)
			http.NotFound(w, r)
			return
		}
		sum := kdf(key, ukek)
		file, err = decrypt(file, sum[0:16], nonce)
		zeroByteArray(sum)
		if err != nil {
			log.Println(err)
			http.NotFound(w, r)
			return
		}
		w.Header().Set("content-type", contentType)
		_, err = w.Write(file)
		if err != nil {
			log.Println(err.Error())
		}
		zeroByteArray(file)
	}
}

func fetchPaste(pasta *Pastae) ([]byte, error) {
	if PASTAELIST == nil {
		return []byte(""), errors.New("PASTAELIST is nil")
	}
	resp, err := decryptPaste(pasta)
	if err != nil {
		return []byte(err.Error()), err
	}
	if pasta.BurnAfterReading {
		PASTAEMUTEX.Lock()
		defer PASTAEMUTEX.Unlock()
		delete(PASTAEMAP, PASTAELIST.Front().Value.(Pastae).ID)
		PASTAELIST.Remove(PASTAELIST.Front())
	}
	return resp, nil
}

func decryptPaste(paste *Pastae) ([]byte, error) {
	sum := kdf(paste.Key, KEK)
	data, err := decrypt(paste.Payload, sum[0:16], paste.Nonce)
	if err != nil {
		return []byte(err.Error()), err
	}
	zeroByteArray(sum)
	return data, nil
}
