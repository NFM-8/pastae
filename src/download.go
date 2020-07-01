package main

import (
	"errors"
	"io/ioutil"
	"log"
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
		}
		w.Header().Set("content-type", data.ContentType)
		w.Write(resp)
		go zeroByteArray(resp, len(resp))
	} else {
		http.NotFound(w, r)
	}
}

func servePasteS(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	id := p.ByName("id")
	pastaeMutex.RLock()
	data, ok := pastaes[id]
	pastaeMutex.RUnlock()
	if ok {
		resp, error := fetchPaste(data)
		if error != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("content-type", data.ContentType)
		w.Write(resp)
		go zeroByteArray(resp, len(resp))
	} else {
		var fname string
		var key []byte
		var nonce []byte
		var uid int64
		var contentType string
		var ukek []byte
		error := db.QueryRow("SELECT fname,key,nonce,uid,ct,kek FROM data,users "+
			"WHERE pid=$1 AND users.id=data.uid", id).Scan(&fname, &key, &nonce, &uid, &contentType, &ukek)
		if error != nil {
			http.NotFound(w, r)
			return
		}
		var file []byte
		file, error = ioutil.ReadFile(configuration.SessionPath + fname)
		if error != nil {
			log.Println(error)
			http.NotFound(w, r)
			return
		}
		sum := kdf(key, ukek)
		file, error = decrypt(file, sum[0:16], nonce)
		go zeroByteArray(sum, 32)
		if error != nil {
			log.Println(error)
			http.NotFound(w, r)
			return
		}
		w.Header().Set("content-type", contentType)
		w.Write(file)
		go zeroByteArray(file, len(file))
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
	sum := kdf(paste.Key, kek)
	data, error := decrypt(paste.Payload, sum[0:16], paste.Nonce)
	if error != nil {
		return []byte(""), error
	}
	go zeroByteArray(sum, 32)
	return data, nil
}
