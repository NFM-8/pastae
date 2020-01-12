package main

import (
	"encoding/hex"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/julienschmidt/httprouter"
)

type Session struct {
	UserID  int64
	Kek     []byte
	Created int64
}

func sessionCleaner(sleepTime time.Duration) {
	for {
		time.Sleep(sleepTime)
		cleanSessions()
	}
}

func cleanSessions() {
	t := time.Now().Unix()
	var expired []string
	sessionMutex.RLock()
	for k, v := range sessions {
		if t-v.Created >= configuration.SessionTimeout {
			expired = append(expired, k)
		}
	}
	sessionMutex.RUnlock()
	sessionMutex.Lock()
	for _, k := range expired {
		delete(sessions, k)
	}
	sessionMutex.Unlock()
}

func expiredCleaner(sleepTime time.Duration) {
	for {
		time.Sleep(sleepTime)
		cleanExpired()
	}
}

func cleanExpired() {
	r, err := db.Query("SELECT id,fname FROM data WHERE expire IS NOT NULL AND expire <= $1",
		time.Now().Unix())
	defer r.Close()
	if err != nil {
		log.Println(err)
		return
	}
	for r.Next() {
		var id int64
		var fname string
		err = r.Scan(&id, &fname)
		if err != nil {
			log.Println(err)
			continue
		}
		db.QueryRow("DELETE FROM data WHERE id = $1", id)
		sessionPasteCountMutex.Lock()
		sessionPasteCount--
		sessionPasteCountMutex.Unlock()
		go func(fname string) {
			err = os.Remove(configuration.SessionPath + fname)
			if err != nil {
				log.Println(err)
			}
		}(fname)
	}
}

func sessionValid(token string) int64 {
	sessionMutex.RLock()
	ses, ok := sessions[token]
	sessionMutex.RUnlock()
	if !ok {
		return -100
	}
	return ses.UserID
}

func registerUserHandler(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	var hash []byte
	hash, err := ioutil.ReadAll(r.Body)
	defer r.Body.Close()
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if registerUser(string(hash)) == "OK" {
		w.Write([]byte("OK"))
	} else {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
}

func registerUser(hash string) string {
	kek, err := generateRandomBytes(64)
	if err != nil {
		log.Println(err)
		return "ERROR"
	}
	_, err = db.Exec("INSERT INTO users (hash,kek) VALUES ($1, $2)", hash, kek)
	if err != nil {
		log.Println(err)
		return "ERROR"
	}
	return "OK"
}

func loginHandler(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	var hash []byte
	hash, err := ioutil.ReadAll(r.Body)
	defer r.Body.Close()
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	var uid int64
	var kek []byte
	err = db.QueryRow("SELECT id, kek FROM users WHERE hash = $1", string(hash)).Scan(&uid, &kek)
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	sidb, _ := generateRandomBytes(64)
	sid := hex.EncodeToString(sidb)
	var sess Session
	sess.Created = time.Now().Unix()
	sess.Kek = kek
	sess.UserID = uid
	sessionMutex.Lock()
	sessions[sid] = sess
	sessionMutex.Unlock()
	w.Write([]byte(sid))
}
