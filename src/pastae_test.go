package main

import (
	"container/list"
	"database/sql"
	"net/http"
	"testing"
	"time"

	"github.com/julienschmidt/httprouter"
)

func TestLRUCache(t *testing.T) {
	CONFIGURATION.MaxEntries = 2
	PASTAEMAP = make(map[string]Pastae)
	PASTAELIST = list.New()
	paste := []byte("Trololooloti")
	contentType := "image/dat"
	var err error
	KEK, err = generateRandomBytes(16)
	if err != nil {
		return
	}
	if len(PASTAEMAP) != PASTAELIST.Len() {
		t.Errorf("Size mismatch")
	}
	id1, err := insertPaste(paste, false, contentType)
	if err != nil || len(PASTAEMAP) != PASTAELIST.Len() {
		t.Errorf("Size mismatch")
	}
	id2, err := insertPaste(paste, false, contentType)
	if err != nil || len(PASTAEMAP) != PASTAELIST.Len() {
		t.Errorf("Size mismatch")
	}
	id3, err := insertPaste(paste, false, contentType)
	if err != nil || len(PASTAEMAP) != PASTAELIST.Len() {
		t.Errorf("Size mismatch")
	}
	id4, err := insertPaste(paste, false, contentType)
	if err != nil || len(PASTAEMAP) != PASTAELIST.Len() {
		t.Errorf("Size mismatch")
	}
	_, ok := PASTAEMAP[id1]
	if ok {
		t.Errorf("Cache clearing failed")
	}
	_, ok = PASTAEMAP[id2]
	if ok {
		t.Errorf("Cache clearing failed")
	}
	_, ok = PASTAEMAP[id3]
	if !ok {
		t.Errorf("Map lookup failed")
	}
	data, ok := PASTAEMAP[id4]
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
	PASTAEMAP = make(map[string]Pastae)
	PASTAELIST = list.New()
	paste := []byte("Trololoo")
	contentType := "wolo/daaddaaaa"
	var err error
	KEK, err = generateRandomBytes(16)
	if err != nil {
		return
	}
	id, err := insertPaste(paste, false, contentType)
	if err != nil {
		t.Error(err)
	}
	data, ok := PASTAEMAP[id]
	if !ok {
		t.Error("Map lookup failed")
	}
	fetched, error := fetchPaste(data)
	if error != nil {
		t.Error("fetchPaste failed")
	}
	if string(fetched) != string(paste) {
		t.Error("Fetched paste is corrupted")
	}
	fetched, error = fetchPaste(data)
	if error != nil {
		t.Error("fetchPaste failed")
	}
	if string(fetched) != string(paste) {
		t.Error("Fetched paste is corrupted")
	}
}

func TestInsertPastaes(t *testing.T) {
	for i := 0; i < 5000; i++ {
		TestInsertPaste(t)
		TestInsertPasteBurnAfterReading(t)
	}
}

func TestInsertPasteBurnAfterReading(t *testing.T) {
	PASTAEMAP = make(map[string]Pastae)
	PASTAELIST = list.New()
	paste := []byte("Wololo")
	contentType := "trolo/daadda"
	var err error
	KEK, err = generateRandomBytes(16)
	if err != nil {
		return
	}
	id, err := insertPaste(paste, true, contentType)
	if err != nil {
		t.Error(err)
	}
	data, ok := PASTAEMAP[id]
	if !ok {
		t.Error("Map lookup failed")
	}
	fetched, error := fetchPaste(data)
	if error != nil {
		t.Error("fetchPaste failed")
	}
	if string(fetched) != string(paste) {
		t.Error("Fetched paste is corrupted")
	}
	_, ok = PASTAEMAP[id]
	if ok {
		t.Error("Paste not burned after fetching")
	}
}

func TestInsertPastaesBurnAfterReading(t *testing.T) {
	for i := 0; i < 1000; i++ {
		TestInsertPasteBurnAfterReading(t)
		TestInsertPaste(t)
	}
}

func TestSessionCleaning(t *testing.T) {
	SESSIONS = make(map[string]Session)
	var session Session
	session.UserID = 765
	session.Created = time.Now().Unix() - 100500100500
	SESSIONS["expired"] = session
	var session2 Session
	session2.UserID = 3124
	session2.Created = time.Now().Unix() + 100500100500
	SESSIONS["valid"] = session2
	cleanSessions()
	_, ok := SESSIONS["expired"]
	if ok {
		t.Error("Expired session not cleaned")
	}
	_, ok = SESSIONS["valid"]
	if !ok {
		t.Error("Valid session cleaned")
	}
}

func TestSessionValidation(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Error(err)
	}
	defer db.Close()
	SESSIONS["sess"] = Session{UserID: 100500, Created: time.Now().Unix()}
	id, _, err := sessionValid(db, "sess")
	if id < 0 || err != nil {
		t.Error("Valid session deemed invalid")
	}
	id, _, err = sessionValid(db, "invalid")
	if id >= 0 || err == nil {
		t.Error("Invalid session deemed valid")
	}
}

func TestSessionCreationTimeUpdate(t *testing.T) {
	SESSIONS["update"] = Session{UserID: 100500, Created: time.Now().Unix() - 1000}
	updateSessionCreationTime("update")
	updated := SESSIONS["update"]
	if updated.Created < time.Now().Unix() {
		t.Error("Session creation time not updated")
	}
}

func TestCreateDbTablesAndIndexes(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Error(err)
	}
	defer db.Close()
	CONFIGURATION.DatabasePersistUser = "TestUser"
	err = createDBTablesAndIndexes(db)
	if err != nil {
		t.Error(err)
	}
}

func TestSessionValidWithPersistUser(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Error(err)
	}
	defer db.Close()
	CONFIGURATION.DatabasePersistUser = "TestUser"
	err = createDBTablesAndIndexes(db)
	if err != nil {
		t.Error(err)
	}

	sid, _, err := sessionValid(db, "")
	if sid < 0 || err != nil {
		t.Error("Persist session not accepted")
	}
	sid, _, err = sessionValid(db, "Invalid")
	if sid >= 0 || err == nil {
		t.Error("Invalid session accepted")
	}
}

func TestRegisterUser(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Error(err)
	}
	defer db.Close()
	CONFIGURATION.DatabasePersistUser = ""
	err = createDBTablesAndIndexes(db)
	if err != nil {
		t.Error(err)
	}

	user := "UseNameAhto"
	err = registerUser(db, user)
	if err != nil {
		t.Error(err)
	}
	err = registerUser(db, user)
	if err == nil {
		t.Error("Duplicate user accepted")
	}
}

func TestSessionValid(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Error(err)
	}
	defer db.Close()
	CONFIGURATION.DatabasePersistUser = ""
	CONFIGURATION.DatabaseTimeout = 36000
	err = createDBTablesAndIndexes(db)
	if err != nil {
		t.Error(err)
	}

	user := "UserSima"
	err = registerUser(db, user)
	if err != nil {
		t.Error(err)
	}
	cleanSessions()
	SESSIONS["bond"] = Session{Created: time.Now().Unix(), Kek: []byte("license to kill"), UserID: 7}
	_, _, err = sessionValid(db, "bond")
	if err != nil {
		t.Error(err)
	}
	cleanSessions()
	_, _, err = sessionValid(db, "bond")
	if err != nil {
		t.Error(err)
	}
	_, _, err = sessionValid(db, "")
	if err == nil {
		t.Error("Invalid session ID accepted")
	}
	_, _, err = sessionValid(db, "bondi")
	if err == nil {
		t.Error("Invalid session ID accepted")
	}

	SESSIONS["Q"] = Session{Created: time.Now().Unix() - 36020, Kek: []byte("Q"), UserID: 10}
	cleanSessions()
	_, _, err = sessionValid(db, "Q")
	if err == nil {
		t.Error("Expired session accepted")
	}
	_, _, err = sessionValid(db, "bond")
	if err != nil {
		t.Error(err)
	}
}

func TestServePaste(t *testing.T) {
	var w http.ResponseWriter
	var r *http.Request = nil
	var p httprouter.Params
	servePaste(w, r, p)
}

func TestServePasteS(t *testing.T) {
	var w http.ResponseWriter
	var r *http.Request = nil
	var p httprouter.Params
	servePasteS(w, r, p)
}

func TestUploadPasteImpl(t *testing.T) {
	var w http.ResponseWriter
	var r *http.Request = nil
	uploadPasteImpl(w, r, true)
	uploadPasteImpl(w, r, false)
}

func TestDeleteHandler(t *testing.T) {
	var w http.ResponseWriter
	var r *http.Request = nil
	var p httprouter.Params
	deleteHandler(w, r, p)
}
