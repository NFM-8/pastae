package main

import (
	"encoding/hex"
	"io"
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
	r, err := db.Query("DELETE FROM data WHERE expire IS NOT NULL AND expire <= $1 RETURNING fname",
		time.Now().Unix())
	defer r.Close()
	if err != nil {
		log.Println(err)
		return
	}
	for r.Next() {
		var fname string
		err = r.Scan(&fname)
		if err != nil {
			log.Println(err)
			continue
		}
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

func sessionValid(token string) (int64, []byte) {
	if token == "" && configuration.SessionPersistUser != "" {
		var uid int64
		var kek []byte
		err := db.QueryRow("SELECT id, kek FROM users WHERE hash = $1",
			configuration.SessionPersistUser).Scan(&uid, &kek)
		if err != nil {
			log.Println(err)
			return -100, []byte("ERR")
		}
		return uid, kek
	}
	sessionMutex.RLock()
	ses, ok := sessions[token]
	sessionMutex.RUnlock()
	if !ok {
		return -100, []byte("ERR")
	}
	return ses.UserID, ses.Kek
}

func registerUserHandler(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	var hash []byte
	hash, err := ioutil.ReadAll(io.LimitReader(r.Body, 100))
	defer r.Body.Close()
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if registerUser(hash) == "OK" {
		w.Write([]byte("OK"))
	} else {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
}

func registerUser(hash []byte) string {
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
	hash, err := ioutil.ReadAll(io.LimitReader(r.Body, 100))
	if string(hash) == configuration.SessionPersistUser {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
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

func logoutHandler(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	var hash []byte
	hash, err := ioutil.ReadAll(io.LimitReader(r.Body, 100))
	defer r.Body.Close()
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusOK)
	go func(hash []byte) {
		sessionMutex.RLock()
		sessid, _ := sessionValid(string(hash))
		sessionMutex.RUnlock()
		if sessid < 0 {
			return
		}
		sessionMutex.Lock()
		delete(sessions, string(hash))
		sessionMutex.Unlock()
	}(hash)
}
