package main

import (
	"bytes"
	"encoding/hex"
	"errors"
	"image"
	"io"
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
		maxEntrySize = CONFIGURATION.DatabaseMaxEntrySize
	} else {
		maxEntrySize = CONFIGURATION.MaxEntrySize
	}
	err := r.ParseMultipartForm(maxEntrySize)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		log.Println("MultiPart parsing")
		return
	}
	var expire int64 = 0
	var uid int64 = 0
	var ukek []byte
	if session {
		uidt, ukekt, err := sessionValid(DB, r.Header.Get("pastae-sessid"))
		if err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		uid = uidt
		ukek = ukekt
		if r.FormValue("expire") == "30" {
			expire = time.Now().Unix() + 30*24*60*60
		}
		if SESSIONPASTECOUNT.Load() >= CONFIGURATION.DatabaseMaxEntries {
			go func() {
				var fname string
				err = DB.QueryRow("DELETE FROM data WHERE id =" +
					"(SELECT id FROM data ORDER BY id LIMIT 1) RETURNING fname").Scan(&fname)
				if err != nil {
					log.Println(err)
					return
				}
				if fname == "" {
					log.Println("empty file name")
					return
				}
				err = os.Remove(CONFIGURATION.DataPath + fname)
				if err != nil {
					log.Println(err)
				}
			}()
		}
	}
	contentType := r.FormValue("content-type")
	bar := r.FormValue("bar") == "bar"
	if contentType == "text/plain" {
		contentType += ";charset=utf-8"
		var id string
		if session {
			if bar {
				id, err = insertPaste([]byte(r.FormValue("data")), bar, contentType)
				if err != nil {
					w.WriteHeader(http.StatusInternalServerError)
					log.Println(err)
					return
				}
			} else {
				id, err = insertPasteToFile([]byte(r.FormValue("data")), contentType, uid, expire, ukek)
				if err != nil {
					w.WriteHeader(http.StatusInternalServerError)
					log.Println(err)
					return
				}
				SESSIONPASTECOUNT.Add(1)
			}
		} else {
			id, err = insertPaste([]byte(r.FormValue("data")), bar, contentType)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				log.Println(err)
				return
			}
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
	if err != nil || header.Size > maxEntrySize {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	data, err := io.ReadAll(io.LimitReader(file, (int64)(maxEntrySize)))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		log.Println("Reading file")
		return
	}
	_, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		log.Println(err)
		return
	}
	contentType = "image/" + format
	var id string
	if session {
		if bar {
			id, err = insertPaste(data, bar, contentType)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				log.Println(err)
				return
			}
		} else {
			id, err = insertPasteToFile(data, contentType, uid, expire, ukek)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				log.Println(err)
				return
			}
			SESSIONPASTECOUNT.Add(1)
		}
	} else {
		id, err = insertPaste(data, bar, contentType)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			log.Println(err)
			return
		}
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

func insertPaste(pasteData []byte, bar bool, contentType string) (string, error) {
	if PASTAELIST == nil {
		return "", errors.New("PASTAELIST is nil")
	}
	PASTAEMUTEX.Lock()
	defer PASTAEMUTEX.Unlock()
	if len(PASTAEMAP) >= CONFIGURATION.MaxEntries {
		if PASTAELIST.Len() > 0 {
			delete(PASTAEMAP, PASTAELIST.Front().Value.(Pastae).Id)
			PASTAELIST.Remove(PASTAELIST.Front())
		}
	}
	nonce, err := generateRandomBytes(12)
	if err != nil {
		return err.Error(), err
	}
	key, error := generateRandomBytes(16)
	if error != nil {
		return err.Error(), err
	}
	pasteData, error = encryptData(pasteData, key, nonce, KEK)
	if error != nil {
		return err.Error(), err
	}
	rnd, error := generateRandomBytes(12)
	if error != nil {
		return err.Error(), err
	}
	id := hex.EncodeToString(rnd)
	if contentType == "text/plain" || contentType == "text/plain;charset=utf-8" {
		id += ".txt"
	} else {
		ct := strings.Split(contentType, "/")
		id += "." + ct[1]
	}
	paste := Pastae{Id: id, BurnAfterReading: bar, ContentType: contentType, Nonce: nonce, Key: key, Payload: pasteData}
	PASTAEMAP[id] = paste
	PASTAELIST.PushBack(paste)
	return CONFIGURATION.URL + id, nil
}

func insertPasteToFile(pasteData []byte,
	contentType string, uid int64, expire int64, ukek []byte) (string, error) {
	rnd, err := generateRandomBytes(12)
	if err != nil {
		return err.Error(), err
	}
	id := hex.EncodeToString(rnd)
	if contentType == "text/plain" || contentType == "text/plain;charset=utf-8" {
		id += ".txt"
	} else {
		ct := strings.Split(contentType, "/")
		id += "." + ct[1]
	}
	rnd, err = generateRandomBytes(12)
	if err != nil {
		return err.Error(), err
	}
	fileName := hex.EncodeToString(rnd)
	if fileName == "" {
		return "Empty file name", errors.New("Empty file name")
	}
	nonce, err := generateRandomBytes(12)
	if err != nil {
		return err.Error(), err
	}
	key, err := generateRandomBytes(16)
	if err != nil {
		return err.Error(), err
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
			_, err := DB.Exec(qs, uid, id, fileName, key, nonce, contentType)
			if err != nil {
				*dbStatus = "ERROR"
				return
			}
			*dbStatus = "OK"
		} else {
			qs := "INSERT INTO data (uid, pid, fname, key, nonce, ct, expire)" +
				"VALUES ($1, $2, $3, $4, $5, $6, $7)"
			_, err := DB.Exec(qs, uid, id, fileName, key, nonce, contentType, expire)
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
		pasteData, err = encryptData(pasteData, key, nonce, ukek)
		if err != nil {
			*fileStatus = err.Error()
			return
		}
		err := os.WriteFile(CONFIGURATION.DataPath+fileName, pasteData, 0644)
		if err != nil {
			*fileStatus = err.Error()
			return
		}
		*fileStatus = "OK"
	}(pasteData, fileName, key, nonce, ukek, &fileStatus)
	wg.Wait()
	if fileStatus != dbStatus || fileStatus != "OK" {
		return "ERROR", errors.New("ERROR")
	}
	return CONFIGURATION.URL + id, nil
}

func deleteHandler(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	sessid := r.Header.Get("pastae-sessid")
	if sessid == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	_, _, err := sessionValid(DB, sessid)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	w.WriteHeader(http.StatusOK)
	go func(pid string) {
		var fname string
		err := DB.QueryRow("DELETE FROM data WHERE pid = $1 RETURNING fname", pid).Scan(&fname)
		if err != nil {
			log.Println(err)
			return
		}
		if fname == "" {
			log.Println("empty file name")
			SESSIONPASTECOUNT.Add(-1)
			return
		}
		err = os.Remove(CONFIGURATION.DataPath + fname)
		if err != nil {
			log.Println(err)
		}
		SESSIONPASTECOUNT.Add(-1)
	}(p.ByName("id"))
}

func encryptData(payload []byte, key []byte, nonce []byte, kek []byte) ([]byte, error) {
	sum := kdf(key, kek)
	payload, err := encrypt(payload, sum[0:16], nonce)
	if err != nil {
		zeroByteArray(sum, 32)
		return nil, err
	}
	zeroByteArray(sum, 32)
	return payload, nil
}
