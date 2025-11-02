package main

import (
	"database/sql"
	"encoding/hex"
	"errors"
	"io"
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
	SESSIONMUTEX.Lock()
	defer SESSIONMUTEX.Unlock()
	for k, v := range SESSIONS {
		if time.Now().Unix()-v.Created >= CONFIGURATION.DatabaseTimeout {
			delete(SESSIONS, k)
		}
	}
}

func expiredCleaner(db *sql.DB, sleepTime time.Duration) {
	if db == nil {
		return
	}
	for {
		time.Sleep(sleepTime)
		cleanExpired(db)
	}
}

func cleanExpired(db *sql.DB) {
	if db == nil {
		return
	}
	r, err := db.Query("DELETE FROM data WHERE expire IS NOT NULL AND expire <= $1 RETURNING fname",
		time.Now().Unix())
	if err != nil {
		ec := r.Close()
		if ec != nil {
			log.Println(ec.Error())
		}
		log.Println(err)
		return
	}
	defer func() {
		ec := r.Close()
		if ec != nil {
			log.Println(ec.Error())
		}
	}()
	for r.Next() {
		SESSIONPASTECOUNT.Add(-1)
		var fname string
		err = r.Scan(&fname)
		if err != nil {
			log.Println(err)
			continue
		}
		err = os.Remove(CONFIGURATION.DataPath + fname)
		if err != nil {
			log.Println(err)
		}
	}
}

func sessionValid(db *sql.DB, token string) (int64, []byte, error) {
	if db == nil {
		return -100, []byte("nil db"), errors.New("nil db")
	}
	if token == "" && CONFIGURATION.DatabasePersistUser != "" {
		var uid int64
		var kek []byte
		err := db.QueryRow("SELECT id, kek FROM users WHERE hash = $1",
			CONFIGURATION.DatabasePersistUser).Scan(&uid, &kek)
		if err != nil {
			return -100, []byte(err.Error()), errors.New("sessionValid")
		}
		return uid, kek, nil
	}
	SESSIONMUTEX.RLock()
	defer SESSIONMUTEX.RUnlock()
	ses, ok := SESSIONS[token]
	if !ok {
		return -100, []byte("Invalid session"), errors.New("sessionValid")
	}
	return ses.UserID, ses.Kek, nil
}

func registerUser(db *sql.DB, hash string) error {
	if db == nil {
		return errors.New("nil db")
	}
	if len(hash) == 0 {
		return errors.New("empty hash")
	}
	kek, err := generateRandomBytes(64)
	if err != nil {
		return err
	}
	_, err = db.Exec("INSERT INTO users (hash,kek) VALUES ($1, $2)", hash, kek)
	if err != nil {
		return err
	}
	return nil
}

func loginHandler(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	if r == nil {
		log.Println("http.Request is nil")
		return
	}
	defer func() {
		ec := r.Body.Close()
		if ec != nil {
			log.Println(ec.Error())
		}
	}()
	hash, err := io.ReadAll(io.LimitReader(r.Body, 100))
	if string(hash) == CONFIGURATION.DatabasePersistUser {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	defer func() {
		ec := r.Body.Close()
		if ec != nil {
			log.Println(ec.Error())
		}
	}()
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	var uid int64
	var kek []byte
	err = DB.QueryRow("SELECT id, kek FROM users WHERE hash = $1", string(hash)).Scan(&uid, &kek)
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	sidb, err := generateRandomBytes(64)
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	sid := hex.EncodeToString(sidb)
	SESSIONMUTEX.Lock()
	defer SESSIONMUTEX.Unlock()
	SESSIONS[sid] = &Session{Created: time.Now().Unix(), Kek: kek, UserID: uid}
	_, err = w.Write([]byte(sid))
	if err != nil {
		log.Println(err.Error())
	}
}

func logoutHandler(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	if r == nil {
		log.Println("http.Request is nil")
		return
	}
	defer func() {
		ec := r.Body.Close()
		if ec != nil {
			log.Println(ec.Error())
		}
	}()
	hash, err := io.ReadAll(io.LimitReader(r.Body, 100))
	defer func() {
		ec := r.Body.Close()
		if ec != nil {
			log.Println(ec.Error())
		}
	}()
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusOK)
	go func(hash []byte) {
		SESSIONMUTEX.Lock()
		defer SESSIONMUTEX.Unlock()
		_, _, err := sessionValid(DB, string(hash))
		if err == nil {
			delete(SESSIONS, string(hash))
		}
	}(hash)
}
