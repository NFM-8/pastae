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
	readConfig()
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

func readConfig() {
	file, err := os.Open("pastae.json")
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()
	c, err := ioutil.ReadAll(file)
	json.Unmarshal(c, &configuration)
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
		} else {
			fmt.Fprint(w, resp)
		}
	}
	http.NotFound(w, r)
}

func fetchPaste(pasta Pastae) (string, error) {
	resp, error := decryptPaste(pasta)
	if error != nil {
		return "ERROR", errors.New("Error fetching paste")
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

}

func insertPaste(pasteData string, bar bool) string {
	if len(pastaes) >= configuration.MaxEntries {
		if firstPastae != nil {
			pastaeMutex.Lock()
			delete(pastaes, firstPastae.Id)
			if firstPastae.Next != nil {
				firstPastae.Next.Prev = nil
				firstPastae = firstPastae.Next
			} else {
				firstPastae = nil
				lastPastae = nil
			}
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
	return id
}

func decryptPaste(paste Pastae) (string, error) {
	data, error := decrypt(paste.Payload, paste.Key, paste.Nonce)
	if error != nil {
		return "", errors.New("Error in decryption")
	}
	return string(data), nil
}
