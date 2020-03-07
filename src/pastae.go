package main

import (
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
	"strconv"

	"github.com/julienschmidt/httprouter"
	_ "github.com/lib/pq"
)

type Configuration struct {
	URL                 string        `json:"url"`
	Listen              string        `json:"listen"`
	FrontPage           string        `json:"frontPage"`
	ReadTimeout         time.Duration `json:"readTimeout"`
	WriteTimeout        time.Duration `json:"writeTimeout"`
	MaxEntries          int           `json:"maxEntries"`
	MaxEntrySize        int64         `json:"maxEntrySize"`
	MaxHeaderBytes      int           `json:"maxHeaderBytes"`
	TLS                 bool          `json:"tls"`
	TLSCert             string        `json:"tlsCert"`
	TLSKey              string        `json:"tlsKey"`
	Session             bool          `json:"session"`
	SessionTimeout      int64         `json:"sessionTimeout"`
	SessionPath         string        `json:"sessionPath"`
	SessionMaxEntries   int64         `json:"sessionMaxEntries"`
	SessionMaxEntrySize int64         `json:"sessionMaxEntrySize"`
	SessionConnStr      string        `json:"sessionConnStr"`
}

type Pastae struct {
	Id               string
	ContentType      string
	BurnAfterReading bool
	Key              []byte
	Nonce            []byte
	Payload          []byte
	Next             *Pastae
	Prev             *Pastae
}

type PastaeListing struct {
	Id          string
	Expire      int64
	ContentType string
}

var configuration Configuration
var pastaes map[string]Pastae
var firstPastae *Pastae
var lastPastae *Pastae
var pastaeMutex sync.RWMutex
var sessionMutex sync.RWMutex
var sessions map[string]Session
var sessionPasteCount int64
var sessionPasteCountMutex sync.RWMutex
var kek []byte
var frontPage []byte
var db *sql.DB

func main() {
	readConfig()
	pastaes = make(map[string]Pastae)
	kekT, error := generateRandomBytes(1024)
	if error != nil {
		log.Fatal(error)
	}
	kek = kekT

	pasteServer := servePaste
	if configuration.Session {
		if _, err := os.Stat(configuration.SessionPath); os.IsNotExist(err) {
			log.Fatal("SessionPath (" + configuration.SessionPath + ") does not exist")
		}
		tdb, err := sql.Open("postgres", configuration.SessionConnStr)
		defer tdb.Close()
		if err != nil {
			log.Fatal(err)
		}
		err = tdb.Ping()
		if err != nil {
			log.Fatal(err)
		}
		db = tdb
		createDbTablesAndIndexes()
		sessions = make(map[string]Session)
		go sessionCleaner(time.Minute)
		go expiredCleaner(time.Minute)
		pasteServer = servePasteS
		l := len(configuration.SessionPath)
		if l > 0 {
			if configuration.SessionPath[l-1] != '/' {
				configuration.SessionPath += "/"
			}
		}
		err = db.QueryRow("SELECT COUNT(id) FROM data").Scan(&sessionPasteCount)
		if err != nil {
			log.Fatal(err)
		}
	}

	mux := httprouter.New()
	mux.GET("/", serveFrontPage)
	mux.GET("/:id", pasteServer)
	mux.POST("/upload", uploadPaste)
	if configuration.Session {
		mux.GET("/list", pasteList)
		mux.POST("/uploadS", uploadPasteS)
		mux.POST("/register", registerUserHandler)
		mux.POST("/login", loginHandler)
		mux.POST("/logout", logoutHandler)
		mux.POST("/:id/expiry/:days", expiry)
		mux.DELETE("/:id", deleteHandler)
	}
	tlsConfig := &tls.Config{PreferServerCipherSuites: true, MinVersion: tls.VersionTLS12}
	s := &http.Server{
		Addr:           configuration.Listen,
		Handler:        mux,
		TLSConfig:      tlsConfig,
		ReadTimeout:    configuration.ReadTimeout * time.Second,
		WriteTimeout:   configuration.WriteTimeout * time.Second,
		MaxHeaderBytes: configuration.MaxHeaderBytes,
	}
	if configuration.TLS {
		log.Fatal(s.ListenAndServeTLS(configuration.TLSCert, configuration.TLSKey))
	} else {
		log.Fatal(s.ListenAndServe())
	}
	return
}

func readConfig() {
	c, err := ioutil.ReadFile("pastae.json")
	if err != nil {
		log.Fatal(err)
	}
	err = json.Unmarshal(c, &configuration)
	if err != nil {
		log.Fatal(err)
	}
	l := len(configuration.URL)
	if l > 0 {
		if configuration.URL[l-1] != '/' {
			configuration.URL += "/"
		}
	}
	frontPage, err = ioutil.ReadFile(configuration.FrontPage)
	if err != nil {
		log.Fatal(err)
	}
}

func serveFrontPage(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	w.Write(frontPage)
}

func pasteList(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	uid, _ := sessionValid(r.Header.Get("pastae-sessid"))
	if uid < 0 {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	res, err := db.Query("SELECT pid,expire,ct FROM data WHERE uid = $1",uid)
	defer res.Close()
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	var resp []PastaeListing
	for res.Next() {
		var elem PastaeListing
		var expireUnix int64
		err = res.Scan(&elem.Id, &expireUnix, &elem.ContentType)
		if err != nil {
			log.Println(err)
			continue
		}
		elem.Expire = expireUnix / (60*60*24)
		resp = append(resp, elem)
	}
	bytes, err := json.Marshal(resp)
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	w.Write(bytes)
}

func expiry(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	uid, _ := sessionValid(r.Header.Get("pastae-sessid"))
	if uid < 0 {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	id := p.ByName("id")
	days, err := strconv.ParseInt(p.ByName("days"), 10, 64)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	t := time.Now().Unix() + days*60*60*24
	_, err = db.Exec("UPDATE data SET expire = $1 WHERE pid = $2 AND uid = $3", t, id, uid)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func createDbTablesAndIndexes() {
	_, err := db.Exec("CREATE UNLOGGED TABLE IF NOT EXISTS users (" +
		"id BIGSERIAL PRIMARY KEY," +
		"hash BYTEA NOT NULL UNIQUE," +
		"kek BYTEA NOT NULL)")
	if err != nil {
		panic(err)
	}
	_, err = db.Exec("CREATE UNLOGGED TABLE IF NOT EXISTS data (" +
		"id BIGSERIAL PRIMARY KEY," +
		"uid BIGINT NOT NULL," +
		"pid VARCHAR NOT NULL," +
		"fname VARCHAR NOT NULL," +
		"key BYTEA NOT NULL," +
		"nonce BYTEA NOT NULL," +
		"ct VARCHAR NOT NULL," +
		"expire BIGINT)")
	if err != nil {
		panic(err)
	}
	_, err = db.Exec("CREATE INDEX IF NOT EXISTS data_uid ON data USING hash (uid)")
	if err != nil {
		panic(err)
	}
	_, err = db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS data_pid ON data USING btree (pid)")
	if err != nil {
		panic(err)
	}
	_, err = db.Exec("CREATE INDEX IF NOT EXISTS data_expire ON data USING btree (expire)")
	if err != nil {
		panic(err)
	}
}
