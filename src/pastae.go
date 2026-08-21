package main

import (
	"container/list"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	_ "github.com/glebarez/go-sqlite"
	"github.com/julienschmidt/httprouter"
)

type Configuration struct {
	URL                  string        `json:"url"`
	Listen               string        `json:"listen"`
	FrontPage            string        `json:"frontPage"`
	ReadTimeout          time.Duration `json:"readTimeout"`
	WriteTimeout         time.Duration `json:"writeTimeout"`
	MaxEntries           int           `json:"maxEntries"`
	MaxEntrySize         int64         `json:"maxEntrySize"`
	MaxHeaderBytes       int           `json:"maxHeaderBytes"`
	TLS                  bool          `json:"tls"`
	TLSCert              string        `json:"tlsCert"`
	TLSKey               string        `json:"tlsKey"`
	DataPath             string        `json:"dataPath"`
	Database             bool          `json:"database"`
	DatabasePersistUser  string        `json:"databasePersistUser"`
	DatabaseTimeout      int64         `json:"databaseTimeout"`
	DatabaseMaxEntries   int64         `json:"databaseMaxEntries"`
	DatabaseMaxEntrySize int64         `json:"databaseMaxEntrySize"`
	DatabaseFile         string        `json:"databaseFile"`
}

type Pastae struct {
	ID               string
	ContentType      string
	BurnAfterReading bool
	Key              []byte
	Nonce            []byte
	Payload          []byte
}

type PastaeListing struct {
	ID          string
	Expire      int64
	ContentType string
}

var CONFIGURATION Configuration
var PASTAEMAP map[string]*Pastae
var PASTAELIST *list.List
var PASTAEMUTEX sync.RWMutex
var SESSIONMUTEX sync.RWMutex
var SESSIONS map[string]*Session
var SESSIONPASTECOUNT atomic.Int64
var KEK []byte
var FRONTPAGE []byte
var DB *sql.DB

func main() {
	err := readConfig("pastae.json")
	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
	PASTAEMAP = make(map[string]*Pastae)
	PASTAELIST = list.New()
	KEK, err = generateRandomBytes(1024)
	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}

	pasteServer := servePaste
	uploadServer := uploadPaste
	if CONFIGURATION.Database {
		if _, err := os.Stat(CONFIGURATION.DataPath); os.IsNotExist(err) {
			slog.Error("SessionPath (" + CONFIGURATION.DataPath + ") does not exist")
			os.Exit(1)
		}
		DB, err = sql.Open("sqlite", CONFIGURATION.DatabaseFile)
		if err != nil {
			slog.Error(err.Error())
			os.Exit(1)
		}
		err = DB.Ping()
		if err != nil {
			slog.Error(err.Error())
			os.Exit(1)
		}
		err = createDBTablesAndIndexes(DB)
		if err != nil {
			slog.Error(err.Error())
			os.Exit(1)
		}
		SESSIONS = make(map[string]*Session)
		go sessionCleaner(time.Minute)
		go expiredCleaner(DB, time.Minute)
		pasteServer = servePasteS
		uploadServer = uploadPasteS
		l := len(CONFIGURATION.DataPath)
		if l > 0 {
			if CONFIGURATION.DataPath[l-1] != '/' {
				CONFIGURATION.DataPath += "/"
			}
		}
		var tmpCount int64 = 0
		err = DB.QueryRow("SELECT COUNT(id) FROM data").Scan(&tmpCount)
		if err != nil {
			slog.Error(err.Error())
			os.Exit(1)
		}
		SESSIONPASTECOUNT.Store(tmpCount)
	}

	mux := httprouter.New()
	mux.GET("/", serveFrontPage)
	mux.GET("/:id", pasteServer)
	mux.POST("/upload", uploadServer)
	if CONFIGURATION.Database {
		mux.POST("/session/list", pasteList)
		mux.POST("/session/register", registerUserHandler)
		mux.POST("/session/login", loginHandler)
		mux.POST("/session/logout", logoutHandler)
		mux.POST("/expiry/:id/:days", expiry)
		mux.POST("/session/ping", pingHandler)
		mux.DELETE("/:id", deleteHandler)
	}
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS13}
	s := &http.Server{
		Addr:           CONFIGURATION.Listen,
		Handler:        mux,
		TLSConfig:      tlsConfig,
		ReadTimeout:    CONFIGURATION.ReadTimeout * time.Second,
		WriteTimeout:   CONFIGURATION.WriteTimeout * time.Second,
		MaxHeaderBytes: CONFIGURATION.MaxHeaderBytes,
	}
	if CONFIGURATION.TLS {
		slog.Error(s.ListenAndServeTLS(CONFIGURATION.TLSCert, CONFIGURATION.TLSKey).Error())
		os.Exit(1)
	} else {
		slog.Error(s.ListenAndServe().Error())
		os.Exit(1)
	}
}

func readConfig(file string) error {
	c, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	err = json.Unmarshal(c, &CONFIGURATION)
	if err != nil {
		return err
	}
	l := len(CONFIGURATION.URL)
	if l > 0 {
		if CONFIGURATION.URL[l-1] != '/' {
			CONFIGURATION.URL += "/"
		}
	}
	FRONTPAGE, err = os.ReadFile(CONFIGURATION.FrontPage)
	if err != nil {
		return err
	}
	return nil
}

func serveFrontPage(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	if r == nil {
		slog.Error("http.Request is nil")
		return
	}
	defer func() {
		ec := r.Body.Close()
		if ec != nil {
			slog.Error(ec.Error())
		}
	}()
	_, err := w.Write(FRONTPAGE)
	if err != nil {
		slog.Error(err.Error())
	}
}

func pasteList(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	if r == nil {
		slog.Error("http.Request is nil")
		return
	}
	defer func() {
		ec := r.Body.Close()
		if ec != nil {
			slog.Error(ec.Error())
		}
	}()
	sessid := r.Header.Get("pastae-sessid")
	if sessid == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	uid, _, err := sessionValid(DB, sessid)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	res, err := DB.Query("SELECT pid,COALESCE(expire,0) as ex,ct FROM data WHERE uid = $1", uid)
	if err != nil {
		ec := res.Close()
		if ec != nil {
			slog.Error(ec.Error())
		}
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	defer func() {
		ec := res.Close()
		if ec != nil {
			slog.Error(ec.Error())
		}
	}()
	var resp []PastaeListing
	for res.Next() {
		var elem PastaeListing
		var expireUnix int64
		err = res.Scan(&elem.ID, &expireUnix, &elem.ContentType)
		if err != nil {
			slog.Error(err.Error())
			continue
		}
		elem.Expire = expireUnix / (60 * 60 * 24)
		resp = append(resp, elem)
	}
	bytes, err := json.Marshal(resp)
	if err != nil {
		slog.Error(err.Error())
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	_, err = w.Write(bytes)
	if err != nil {
		slog.Error(err.Error())
	}
}

func expiry(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	if r == nil {
		slog.Error("http.Request is nil")
		return
	}
	defer func() {
		ec := r.Body.Close()
		if ec != nil {
			slog.Error(ec.Error())
		}
	}()
	sessid := r.Header.Get("pastae-sessid")
	if sessid == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	uid, _, err := sessionValid(DB, sessid)
	if err != nil {
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
	_, err = DB.Exec("UPDATE data SET expire = $1 WHERE pid = $2 AND uid = $3", t, id, uid)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func pingHandler(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	defer func() {
		ec := r.Body.Close()
		if ec != nil {
			slog.Error(ec.Error())
		}
	}()
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
	updateSessionCreationTime(sessid)
	w.WriteHeader(http.StatusOK)
}

func updateSessionCreationTime(sessid string) {
	SESSIONMUTEX.Lock()
	defer SESSIONMUTEX.Unlock()
	sessionData := SESSIONS[sessid]
	sessionData.Created = time.Now().Unix()
	SESSIONS[sessid] = sessionData
}

func createDBTablesAndIndexes(db *sql.DB) error {
	_, err := db.Exec("CREATE TABLE IF NOT EXISTS users (" +
		"id INTEGER PRIMARY KEY," +
		"hash TEXT NOT NULL UNIQUE," +
		"kek BLOB NOT NULL)")
	if err != nil {
		return err
	}
	_, err = db.Exec("CREATE TABLE IF NOT EXISTS data (" +
		"id INTEGER PRIMARY KEY," +
		"uid INTEGER NOT NULL," +
		"pid TEXT NOT NULL," +
		"fname TEXT NOT NULL," +
		"key BLOB NOT NULL," +
		"nonce BLOB NOT NULL," +
		"ct TEXT NOT NULL," +
		"expire INTEGER)")
	if err != nil {
		return err
	}
	_, err = db.Exec("CREATE INDEX IF NOT EXISTS data_uid ON data (uid)")
	if err != nil {
		return err
	}
	_, err = db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS data_pid ON data (pid)")
	if err != nil {
		return err
	}
	_, err = db.Exec("CREATE INDEX IF NOT EXISTS data_expire ON data (expire)")
	if err != nil {
		return err
	}
	if CONFIGURATION.DatabasePersistUser != "" {
		var kek []byte
		err = db.QueryRow("SELECT kek FROM users WHERE hash=$1", CONFIGURATION.DatabasePersistUser).Scan(&kek)
		if err != nil {
			err = registerUser(db, CONFIGURATION.DatabasePersistUser)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func registerUserHandler(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	if r == nil {
		slog.Error("http.Request is nil")
		return
	}
	defer func() {
		ec := r.Body.Close()
		if ec != nil {
			slog.Error(ec.Error())
		}
	}()
	hash, err := io.ReadAll(io.LimitReader(r.Body, 100))
	defer func() {
		ec := r.Body.Close()
		if ec != nil {
			slog.Error(ec.Error())
		}
	}()
	if err != nil {
		slog.Error(err.Error())
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	err = registerUser(DB, string(hash))
	if err == nil {
		_, err = w.Write([]byte("OK"))
		if err != nil {
			slog.Error(err.Error())
		}
	} else {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
}
