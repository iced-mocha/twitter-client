package session

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"
)

var (
	mu       sync.Mutex
	sessions = make(map[string]map[string]interface{})
)

// Get returns the session data for the request client.
func Get(r *http.Request) (s map[string]interface{}) {
	if c, _ := r.Cookie("mochaTwitter"); c != nil && c.Value != "" {
		mu.Lock()
		s = sessions[c.Value]
		mu.Unlock()
	}
	if s == nil {
		s = make(map[string]interface{})
	}
	return s
}

// Save saves session for the request client.
func Save(w http.ResponseWriter, r *http.Request, s map[string]interface{}) error {
	key := ""
	if c, _ := r.Cookie("mochaTwitter"); c != nil {
		key = c.Value
	}
	if len(s) == 0 {
		if key != "" {
			mu.Lock()
			delete(sessions, key)
			mu.Unlock()
		}
		return nil
	}
	if key == "" {
		var buf [16]byte
		_, err := rand.Read(buf[:])
		if err != nil {
			return err
		}
		key = hex.EncodeToString(buf[:])
		http.SetCookie(w, &http.Cookie{
			Name:     "session",
			Path:     "/",
			HttpOnly: true,
			Value:    key,
		})
	}
	mu.Lock()
	sessions[key] = s
	mu.Unlock()
	return nil
}
