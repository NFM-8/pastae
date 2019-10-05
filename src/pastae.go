package main

import (
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/julienschmidt/httprouter"
)

type Configuration struct {
	Path           string
	Port           string
	ReadTimeout    time.Duration
	WriteTimeout   time.Duration
	MaxHeaderBytes int
	TLS            bool
}

type Pastae struct {
	BurnAfterReading bool
	Key              []byte
	Nonce            []byte
	Payload          []byte
}

var configuration Configuration
var pastaes map[string]Pastae
var pastaeMutex sync.Mutex

func main() {
	pastaes = make(map[string]Pastae)
	mux := httprouter.New()
	mux.GET("/", serveRoot)
	mux.GET("/paste/:id", servePaste)
	mux.PUT("/paste/upload", uploadPaste)
	s := &http.Server{
		Addr:           configuration.Port,
		Handler:        mux,
		ReadTimeout:    configuration.ReadTimeout,
		WriteTimeout:   configuration.WriteTimeout,
		MaxHeaderBytes: configuration.MaxHeaderBytes,
	}
	log.Fatal(s.ListenAndServe())
	return
}

func serveRoot(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	fmt.Fprint(w, "Welcome to Pastae")
}

func servePaste(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	id := p.ByName("id")
	data, ok := pastaes[id]
	if ok {
		resp, error := fetchPaste(data)
		if error != nil {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, resp)
		if data.BurnAfterReading {
			delete(pastaes, id)
		}
		return
	}
	http.NotFound(w, r)
}

func uploadPaste(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {

}

func insertPaste(pasteData string, bar bool) string {
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
	paste.Nonce = nonce
	paste.Key = key
	payload, error := encrypt([]byte(pasteData), paste.Key, paste.Nonce)
	if error != nil {
		return "ERROR"
	}
	paste.Payload = payload
	rnd, error := generateRandomBytes(12)
	if error != nil {
		return "ERROR"
	}
	id := hex.EncodeToString(rnd)
	pastaeMutex.Lock()
	pastaes[id] = paste
	pastaeMutex.Unlock()
	return id
}

func fetchPaste(paste Pastae) (string, error) {
	data, error := decrypt(paste.Payload, paste.Key, paste.Nonce)
	if error != nil {
		return "", errors.New("Error in decryption")
	}
	return string(data), nil
}
