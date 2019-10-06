package main

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sync"

	"github.com/julienschmidt/httprouter"
)

type Configuration struct {
	Path string `json:"path"`
	Addr string `json:"addr"`
	//ReadTimeout    time.Duration
	//WriteTimeout   time.Duration
	MaxEntries     int  `json:"maxEntries"`
	MaxEntrySize   int  `json:"maxEntrySize"`
	MaxHeaderBytes int  `json:"maxHeaderBytes"`
	TLS            bool `json:"tls"`
}

type Pastae struct {
	Id               string
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
var pastaeMutex sync.Mutex

func main() {
	// Read config file
	file, err := os.Open("pastae.json")
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()
	c, err := ioutil.ReadAll(file)
	json.Unmarshal(c, &configuration)

	pastaes = make(map[string]Pastae)
	mux := httprouter.New()
	mux.GET("/", serveRoot)
	mux.GET("/paste/:id", servePaste)
	mux.PUT("/paste/upload", uploadPaste)
	s := &http.Server{
		Addr:    configuration.Addr,
		Handler: mux,
		//ReadTimeout:    configuration.ReadTimeout,
		//WriteTimeout:   configuration.WriteTimeout,
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
			pasta := pastaes[id]
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
			delete(pastaes, id)
			pastaeMutex.Unlock()
		}
		return
	}
	http.NotFound(w, r)
}

func uploadPaste(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {

}

func insertPaste(pasteData string, bar bool) string {
	if len(pastaes) >= configuration.MaxEntries {
		if firstPastae != nil {
			pastaeMutex.Lock()
			delete(pastaes, firstPastae.Id)
			firstPastae.Next.Prev = nil
			firstPastae = firstPastae.Next
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
	paste.Next = nil
	paste.Prev = nil
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
	return id
}

func fetchPaste(paste Pastae) (string, error) {
	data, error := decrypt(paste.Payload, paste.Key, paste.Nonce)
	if error != nil {
		return "", errors.New("Error in decryption")
	}
	return string(data), nil
}
