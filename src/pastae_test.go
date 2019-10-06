package main

import (
	"testing"
)

func TestInsertPaste(t *testing.T) {
	pastaes = make(map[string]Pastae)
	paste := []byte("Trololoo")
	id := insertPaste(paste, false)
	data, ok := pastaes[id]
	if !ok {
		t.Errorf("Map lookup failed")
	}
	fetched, error := fetchPaste(data)
	if error != nil {
		t.Errorf("fetchPaste failed")
	}
	if fetched != string(paste) {
		t.Errorf("Fetched paste is corrupted")
	}
	fetched, error = fetchPaste(data)
	if error != nil {
		t.Errorf("fetchPaste failed")
	}
	if fetched != string(paste) {
		t.Errorf("Fetched paste is corrupted")
	}
}

func TestInsertPasteBurnAfterReading(t *testing.T) {
	pastaes = make(map[string]Pastae)
	paste := []byte("Wololoo")
	id := insertPaste(paste, true)
	data, ok := pastaes[id]
	if !ok {
		t.Errorf("Map lookup failed")
	}
	fetched, error := fetchPaste(data)
	if error != nil {
		t.Errorf("fetchPaste failed")
	}
	if fetched != string(paste) {
		t.Errorf("Fetched paste is corrupted")
	}
	data, ok = pastaes[id]
	if ok {
		t.Errorf("Paste not burned after fetching")
	}
}

func TestLRUCache(t *testing.T) {
	configuration.MaxEntries = 2
	pastaes = make(map[string]Pastae)
	paste := []byte("Trololoo")
	id1 := insertPaste(paste, false)
	id2 := insertPaste(paste, false)
	id3 := insertPaste(paste, false)
	id4 := insertPaste(paste, false)
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
	if fetched != string(paste) {
		t.Errorf("Fetched paste is corrupted")
	}
	fetched, error = fetchPaste(data)
	if error != nil {
		t.Errorf("fetchPaste failed")
	}
	if fetched != string(paste) {
		t.Errorf("Fetched paste is corrupted")
	}
}
