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

var configuration Configuration
var pastaes map[string]Pastae
var firstPastae *Pastae
var lastPastae *Pastae
var pastaeMutex sync.RWMutex
var sessionMutex sync.RWMutex
var sessions map[string]Session
var kek []byte
var frontPage []byte
var db *sql.DB

func main() {
	readConfig()
	pastaes = make(map[string]Pastae)
	kekT, error := generateRandomBytes(1024)
	if error != nil {
		panic(error)
	}
	kek = kekT

	if configuration.Session {
		if _, err := os.Stat(configuration.SessionPath); os.IsNotExist(err) {
			log.Fatal("SessionPath does not exist")
		}
		tdb, err := sql.Open("postgres", configuration.SessionConnStr)
		defer tdb.Close()
		if err != nil {
			panic(err)
		}
		err = tdb.Ping()
		if err != nil {
			panic(err)
		}
		db = tdb
		createDbTablesAndIndexes()
		sessions = make(map[string]Session)
		go sessionCleaner(time.Minute)
	}

	mux := httprouter.New()
	mux.GET("/", serveFrontPage)
	mux.GET("/:id", servePaste)
	mux.POST("/upload", uploadPaste)
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
	json.Unmarshal(c, &configuration)
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

func createDbTablesAndIndexes() {
	_, err := db.Exec("CREATE UNLOGGED TABLE IF NOT EXISTS users (" +
		"id BIGINT PRIMARY KEY," +
		"hash BYTEA NOT NULL," +
		"kek BYTEA NOT NULL)")
	if err != nil {
		panic(err)
	}
	_, err = db.Exec("CREATE UNLOGGED TABLE IF NOT EXISTS data (" +
		"id BIGINT PRIMARY KEY," +
		"uid BIGINT NOT NULL," +
		"pid VARCHAR NOT NULL," +
		"fname VARCHAR NOT NULL," +
		"key BYTEA NOT NULL," +
		"nonce BYTEA NOT NULL," +
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
