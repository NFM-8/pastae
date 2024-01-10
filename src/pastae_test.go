package main

import (
	"container/list"
	"database/sql"
	"testing"
	"time"
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
		t.Errorf(err.Error())
	}
	data, ok := PASTAEMAP[id]
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
		t.Errorf(err.Error())
	}
	data, ok := PASTAEMAP[id]
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
	_, ok = PASTAEMAP[id]
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
		t.Errorf("Expired session not cleaned")
	}
	_, ok = SESSIONS["valid"]
	if !ok {
		t.Errorf("Valid session cleaned")
	}
}

func TestSessionValidation(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Errorf(err.Error())
	}
	defer db.Close()
	SESSIONS["sess"] = Session{UserID: 100500, Created: time.Now().Unix()}
	id, _, err := sessionValid(db, "sess")
	if id < 0 || err != nil {
		t.Errorf("Valid session deemed invalid")
	}
	id, _, err = sessionValid(db, "invalid")
	if id >= 0 || err == nil {
		t.Errorf("Invalid session deemed valid")
	}
}

func TestSessionCreationTimeUpdate(t *testing.T) {
	var session Session
	session.UserID = 100500
	session.Created = time.Now().Unix() - 1000
	SESSIONS["update"] = session
	updateSessionCreationTime("update")
	updated := SESSIONS["update"]
	if updated.Created < time.Now().Unix() {
		t.Errorf("Session creation time not updated")
	}
}

func TestCreateDbTablesAndIndexes(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Errorf(err.Error())
	}
	defer db.Close()
	CONFIGURATION.DatabasePersistUser = "TestUser"
	err = createDbTablesAndIndexes(db)
	if err != nil {
		t.Errorf(err.Error())
	}
}

func TestSessionValidWithPersistUser(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Errorf(err.Error())
	}
	defer db.Close()
	CONFIGURATION.DatabasePersistUser = "TestUser"
	err = createDbTablesAndIndexes(db)
	if err != nil {
		t.Errorf(err.Error())
	}

	sid, _, err := sessionValid(db, "")
	if sid < 0 || err != nil {
		t.Errorf("Persist session not accepted")
	}
	sid, _, err = sessionValid(db, "Invalid")
	if sid >= 0 || err == nil {
		t.Errorf("Invalid session accepted")
	}
}

func TestRegisterUser(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Errorf(err.Error())
	}
	defer db.Close()
	CONFIGURATION.DatabasePersistUser = ""
	err = createDbTablesAndIndexes(db)
	if err != nil {
		t.Errorf(err.Error())
	}

	user := "UseNameAhto"
	err = registerUser(db, user)
	if err != nil {
		t.Errorf(err.Error())
	}
	err = registerUser(db, user)
	if err == nil {
		t.Errorf("Duplicate user accepted")
	}
}

func TestSessionValid(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Errorf(err.Error())
	}
	defer db.Close()
	CONFIGURATION.DatabasePersistUser = ""
	CONFIGURATION.DatabaseTimeout = 36000
	err = createDbTablesAndIndexes(db)
	if err != nil {
		t.Errorf(err.Error())
	}

	user := "UserSima"
	err = registerUser(db, user)
	if err != nil {
		t.Errorf(err.Error())
	}
	cleanSessions()
	SESSIONS["bond"] = Session{Created: time.Now().Unix(), Kek: []byte("license to kill"), UserID: 7}
	_, _, err = sessionValid(db, "bond")
	if err != nil {
		t.Errorf(err.Error())
	}
	cleanSessions()
	_, _, err = sessionValid(db, "bond")
	if err != nil {
		t.Errorf(err.Error())
	}
	_, _, err = sessionValid(db, "")
	if err == nil {
		t.Errorf("Invalid session ID accepted")
	}
	_, _, err = sessionValid(db, "bondi")
	if err == nil {
		t.Errorf("Invalid session ID accepted")
	}

	SESSIONS["Q"] = Session{Created: time.Now().Unix() - 36020, Kek: []byte("Q"), UserID: 10}
	cleanSessions()
	_, _, err = sessionValid(db, "Q")
	if err == nil {
		t.Errorf("Expired session accepted")
	}
	_, _, err = sessionValid(db, "bond")
	if err != nil {
		t.Errorf(err.Error())
	}
}
