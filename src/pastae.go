package main

import (
	"crypto/sha512"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/julienschmidt/httprouter"
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

type Session struct {
	UserID  int64
	Kek     []byte
	Created int64
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

func main() {
	readConfig()
	pastaes = make(map[string]Pastae)
	kekT, error := generateRandomBytes(1024)
	if error != nil {
		return
	}
	kek = kekT

	if configuration.Session {
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
		} else {
			w.Header().Set("content-type", data.ContentType)
			w.Write(resp)
			for i := 0; i < len(resp); i++ {
				resp[i] = 0
			}
		}
	} else {
		http.NotFound(w, r)
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

func uploadPaste(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	err := r.ParseMultipartForm(configuration.MaxEntrySize + 4096)
	if err != nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	contentType := r.FormValue("content-type")
	bar := r.FormValue("bar") == "bar"
	if contentType == "text/plain" {
		id := insertPaste([]byte(r.FormValue("data")), bar, contentType)
		if err := r.Body.Close(); err != nil {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(id))
		return
	}
	if contentType == "image/jpeg" || contentType == "image/png" {
		file, header, err := r.FormFile("file")
		if err != nil || header.Size > configuration.MaxEntrySize {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		data, err := ioutil.ReadAll(io.LimitReader(file, configuration.MaxEntrySize))
		if err != nil {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		id := insertPaste(data, bar, contentType)
		if err := r.Body.Close(); err != nil {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(id))
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
	var ekey [32]byte
	for i := 0; i < 16; i++ {
		ekey[i] = paste.Key[i]
	}
	for i := 16; i < 32; i++ {
		ekey[i] = kek[i-16]
	}
	sum := sha512.Sum512(ekey[0:32])
	payload, error := encrypt(pasteData, sum[0:16], paste.Nonce)
	if error != nil {
		return "ERROR"
	}
	for i := 0; i < 32; i++ {
		sum[i] = 0
	}
	paste.Payload = payload
	rnd, error := generateRandomBytes(12)
	if error != nil {
		return "ERROR"
	}
	id := hex.EncodeToString(rnd)
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

func decryptPaste(paste Pastae) ([]byte, error) {
	var key [32]byte
	for i := 0; i < 16; i++ {
		key[i] = paste.Key[i]
	}
	for i := 16; i < 32; i++ {
		key[i] = kek[i-16]
	}
	sum := sha512.Sum512(key[0:32])
	data, error := decrypt(paste.Payload, sum[0:16], paste.Nonce)
	if error != nil {
		return []byte(""), errors.New("Error in decryption")
	}
	for i := 0; i < 32; i++ {
		sum[i] = 0
	}
	return data, nil
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

func sessionValid(token string) (id int64) {
	sessionMutex.RLock()
	ses, ok := sessions[token]
	if !ok {
		sessionMutex.RUnlock()
		return -100
	}
	sessionMutex.RUnlock()
	return ses.UserID
}
