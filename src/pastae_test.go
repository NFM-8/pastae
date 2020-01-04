package main

import (
	"testing"
)

func TestLRUCache(t *testing.T) {
	configuration.MaxEntries = 2
	pastaes = make(map[string]Pastae)
	paste := []byte("Trololooloti")
	contentType := "dat"
	kekT, error := generateRandomBytes(16)
	if error != nil {
		return
	}
	kek = kekT
	id1 := insertPaste(paste, false, contentType)
	id2 := insertPaste(paste, false, contentType)
	id3 := insertPaste(paste, false, contentType)
	id4 := insertPaste(paste, false, contentType)
	data, ok := pastaes[id1]
	if ok {
		t.Errorf("Cache clearing failed")
	}
	data, ok = pastaes[id2]
	if ok {
		t.Errorf("Cache clearing failed")
	}
	data, ok = pastaes[id3]
	if !ok {
		t.Errorf("Map lookup failed")
	}
	data, ok = pastaes[id4]
	if !ok {
		t.Errorf("Map lookup failed")
	}
	fetched, error := fetchPaste(data)
	if error != nil {
		t.Errorf("fetchPaste failed")
	}
	if string(fetched) != string(paste) {
		t.Errorf("Fetched paste is corrupted")
	}
	fetched, error = fetchPaste(data)
	if error != nil {
		t.Errorf("fetchPaste failed")
	}
	if string(fetched) != string(paste) {
		t.Errorf("Fetched paste is corrupted")
	}
}

func TestInsertPaste(t *testing.T) {
	pastaes = make(map[string]Pastae)
	paste := []byte("Trololoo")
	contentType := "daaddaaaa"
	kekT, error := generateRandomBytes(16)
	if error != nil {
		return
	}
	kek = kekT
	id := insertPaste(paste, false, contentType)
	data, ok := pastaes[id]
	if !ok {
		t.Errorf("Map lookup failed")
	}
	fetched, error := fetchPaste(data)
	if error != nil {
		t.Errorf("fetchPaste failed")
	}
	if string(fetched) != string(paste) {
		t.Errorf("Fetched paste is corrupted")
	}
	fetched, error = fetchPaste(data)
	if error != nil {
		t.Errorf("fetchPaste failed")
	}
	if string(fetched) != string(paste) {
		t.Errorf("Fetched paste is corrupted")
	}
}

func TestInsertPastaes(t *testing.T) {
	for i := 0; i < 5000; i++ {
		TestInsertPaste(t)
		TestInsertPasteBurnAfterReading(t)
	}
}

func TestInsertPasteBurnAfterReading(t *testing.T) {
	pastaes = make(map[string]Pastae)
	paste := []byte("Wololo")
	contentType := "daadda"
	kekT, error := generateRandomBytes(16)
	if error != nil {
		return
	}
	kek = kekT
	id := insertPaste(paste, true, contentType)
	data, ok := pastaes[id]
	if !ok {
		t.Errorf("Map lookup failed")
	}
	fetched, error := fetchPaste(data)
	if error != nil {
		t.Errorf("fetchPaste failed")
	}
	if string(fetched) != string(paste) {
		t.Errorf("Fetched paste is corrupted")
	}
	data, ok = pastaes[id]
	if ok {
		t.Errorf("Paste not burned after fetching")
	}
}

func TestInsertPastaesBurnAfterReading(t *testing.T) {
	for i := 0; i < 1000; i++ {
		TestInsertPasteBurnAfterReading(t)
		TestInsertPaste(t)
	}
}
