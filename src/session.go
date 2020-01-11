package main

import "time"

type Session struct {
	UserID  int64
	Kek     []byte
	Created int64
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
