package main

import (
	"bytes"
	"encoding/hex"
	"errors"
	"image"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	_ "golang.org/x/image/webp"

	"github.com/julienschmidt/httprouter"
)

func uploadPaste(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	uploadPasteImpl(w, r, false)
}

func uploadPasteS(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	uploadPasteImpl(w, r, true)
}

func uploadPasteImpl(w http.ResponseWriter, r *http.Request, session bool) {
	var maxEntrySize int64
	if session {
		maxEntrySize = configuration.SessionMaxEntrySize
	} else {
		maxEntrySize = configuration.MaxEntrySize
	}
	err := r.ParseMultipartForm(maxEntrySize)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		log.Println("MultiPart parsing")
		return
	}
	var uid int64 = 0
	var expire int64 = 0
	var ukek []byte
	if session {
		uid, ukek = sessionValid(r.Header.Get("pastae-sessid"))
		if uid < 0 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.FormValue("expire") == "30" {
			expire = time.Now().Unix() + 30*24*60*60
		}
		var pcount int64
		sessionPasteCountMutex.RLock()
		pcount = sessionPasteCount
		sessionPasteCountMutex.RUnlock()
		if pcount >= configuration.SessionMaxEntries {
			go func() {
				var fname string
				err = db.QueryRow("DELETE FROM data WHERE id =" +
					"(SELECT id FROM data ORDER BY id LIMIT 1) RETURNING fname").Scan(&fname)
				if err != nil {
					log.Println(err)
				}
				err = os.Remove(configuration.SessionPath + fname)
				if err != nil {
					log.Println(err)
				}
			}()
		}
	}
	contentType := r.FormValue("content-type")
	bar := r.FormValue("bar") == "bar"
	if contentType == "text/plain" {
		var id string
		if session {
			if bar {
				id = insertPaste([]byte(r.FormValue("data")), bar, contentType)
			} else {
				id = insertPasteToFile([]byte(r.FormValue("data")), contentType, uid, expire, ukek)
			}
			sessionPasteCountMutex.Lock()
			sessionPasteCount++
			sessionPasteCountMutex.Unlock()
		} else {
			id = insertPaste([]byte(r.FormValue("data")), bar, contentType)
		}
		if err := r.Body.Close(); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			log.Println("Body close")
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(id))
		return
	}
	file, header, err := r.FormFile("file")
	var format string
	if err != nil || header.Size > maxEntrySize {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	data, err := ioutil.ReadAll(io.LimitReader(file, maxEntrySize))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		log.Println("Reading file")
		return
	}
	_, format, err = image.Decode(bytes.NewReader(data))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		log.Println(err)
		return
	}
	contentType = "image/" + format
	var id string
	if session {
		if bar {
			id = insertPaste(data, bar, contentType)
		} else {
			id = insertPasteToFile(data, contentType, uid, expire, ukek)
		}
		sessionPasteCountMutex.Lock()
		sessionPasteCount++
		sessionPasteCountMutex.Unlock()
	} else {
		id = insertPaste(data, bar, contentType)
	}
	if err := r.Body.Close(); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		log.Println("Body close")
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(id))
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
	pasteData, error = encryptData(pasteData, key, nonce, kek)
	if error != nil {
		return "ERROR"
	}
	paste.Payload = pasteData
	rnd, error := generateRandomBytes(12)
	if error != nil {
		return "ERROR"
	}
	id := hex.EncodeToString(rnd)
	if contentType == "text/plain" {
		id += ".txt"
	} else {
		ct := strings.Split(contentType, "/")
		id += "." + ct[1]
	}
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

func insertPasteToFile(pasteData []byte,
	contentType string, uid int64, expire int64, ukek []byte) string {
	rnd, error := generateRandomBytes(12)
	if error != nil {
		return "ERROR"
	}
	id := hex.EncodeToString(rnd)
	if contentType == "text/plain" {
		id += ".txt"
	} else {
		ct := strings.Split(contentType, "/")
		id += "." + ct[1]
	}
	rnd, error = generateRandomBytes(12)
	if error != nil {
		return "ERROR"
	}
	fileName := hex.EncodeToString(rnd)
	nonce, error := generateRandomBytes(12)
	if error != nil {
		return "ERROR"
	}
	key, error := generateRandomBytes(16)
	if error != nil {
		return "ERROR"
	}
	var dbStatus string
	var fileStatus string
	var wg sync.WaitGroup
	wg.Add(1)
	go func(uid int64, id string, fileName string, key []byte, nonce []byte, ct string, dbStatus *string) {
		defer wg.Done()
		if expire == 0 {
			qs := "INSERT INTO data (uid, pid, fname, key, nonce, ct)" +
				"VALUES ($1, $2, $3, $4, $5, $6)"
			_, err := db.Exec(qs, uid, id, fileName, key, nonce, contentType)
			if err != nil {
				*dbStatus = "ERROR"
				return
			}
			*dbStatus = "OK"
		} else {
			qs := "INSERT INTO data (uid, pid, fname, key, nonce, ct, expire)" +
				"VALUES ($1, $2, $3, $4, $5, $6, $7)"
			_, err := db.Exec(qs, uid, id, fileName, key, nonce, contentType, expire)
			if err != nil {
				*dbStatus = "ERROR"
				return
			}
			*dbStatus = "OK"
		}
	}(uid, id, fileName, key, nonce, contentType, &dbStatus)
	wg.Add(1)
	go func(pasteData []byte, fileName string, key []byte, nonce []byte, ukek []byte, fileStatus *string) {
		defer wg.Done()
		pasteData, error = encryptData(pasteData, key, nonce, ukek)
		if error != nil {
			*fileStatus = "ERROR"
			return
		}
		err := ioutil.WriteFile(configuration.SessionPath+fileName, pasteData, 0644)
		if err != nil {
			*fileStatus = "ERROR"
			return
		}
		*fileStatus = "OK"
	}(pasteData, fileName, key, nonce, ukek, &fileStatus)
	wg.Wait()
	if fileStatus != dbStatus || fileStatus != "OK" {
		return "ERROR"
	}
	return configuration.URL + id
}

func deleteHandler(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	sessid := r.Header.Get("pastae-sessid")
	if sessid == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	uid, _ := sessionValid(sessid)
	if uid < 0 {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	w.WriteHeader(http.StatusOK)
	go func(pid string) {
		var fname string
		err := db.QueryRow("DELETE FROM data WHERE pid = $1 RETURNING fname", pid).Scan(&fname)
		if err != nil {
			log.Println(err)
			return
		}
		err = os.Remove(configuration.SessionPath + fname)
		if err != nil {
			log.Println(err)
		}
		sessionPasteCountMutex.Lock()
		sessionPasteCount--
		sessionPasteCountMutex.Unlock()
	}(p.ByName("id"))
}

func encryptData(payload []byte, key []byte, nonce []byte, kek []byte) ([]byte, error) {
	sum := kdf(key, kek)
	var error error
	payload, error = encrypt(payload, sum[0:16], nonce)
	if error != nil {
		go zeroByteArray(sum, 32)
		return nil, errors.New("ERROR")
	}
	go zeroByteArray(sum, 32)
	return payload, nil
}
