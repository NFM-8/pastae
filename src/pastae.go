package main

import (
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sync"

	"github.com/julienschmidt/httprouter"
)

type Configuration struct {
	URL    string `json:"url"`
	Listen string `json:"listen"`
	//ReadTimeout    time.Duration
	//WriteTimeout   time.Duration
	MaxEntries     int    `json:"maxEntries"`
	MaxEntrySize   int64  `json:"maxEntrySize"`
	MaxHeaderBytes int    `json:"maxHeaderBytes"`
	TLS            bool   `json:"tls"`
	TLSCert        string `json:"tlsCert"`
	TLSKey         string `json:"tlsKey"`
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

func main() {
	readConfig()
	pastaes = make(map[string]Pastae)
	mux := httprouter.New()
	mux.GET("/:id", servePaste)
	mux.PUT("/upload", uploadPaste)
	mux.PUT("/uploadBurning", uploadPasteBurning)
	tlsConfig := &tls.Config{PreferServerCipherSuites: true, MinVersion: tls.VersionTLS12}
	s := &http.Server{
		Addr:      configuration.Listen,
		Handler:   mux,
		TLSConfig: tlsConfig,
		//ReadTimeout:    configuration.ReadTimeout,
		//WriteTimeout:   configuration.WriteTimeout,
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
	file, err := os.Open("pastae.json")
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()
	c, err := ioutil.ReadAll(file)
	json.Unmarshal(c, &configuration)
	l := len(configuration.URL)
	if l > 0 {
		if configuration.URL[l-1] != '/' {
			configuration.URL += "/"
		}
	}
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
			fmt.Fprint(w, resp)
		}
	} else {
		http.NotFound(w, r)
	}
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
	body, err := ioutil.ReadAll(io.LimitReader(r.Body, configuration.MaxEntrySize))
	if err != nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	id := insertPaste(body, false, r.Header.Get("content-type"))
	if err := r.Body.Close(); err != nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(id))
}

func uploadPasteBurning(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	body, err := ioutil.ReadAll(io.LimitReader(r.Body, configuration.MaxEntrySize))
	if err != nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	id := insertPaste(body, true, r.Header.Get("content-type"))
	if err := r.Body.Close(); err != nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(id))
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
	payload, error := encrypt(pasteData, paste.Key, paste.Nonce)
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
	return configuration.URL + id
}

func decryptPaste(paste Pastae) (string, error) {
	data, error := decrypt(paste.Payload, paste.Key, paste.Nonce)
	if error != nil {
		return "", errors.New("Error in decryption")
	}
	return string(data), nil
}
