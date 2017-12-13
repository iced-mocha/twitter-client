package session

import (
	"sync"
)

var (
	mu       sync.Mutex
	sessions = make(map[string]map[string]interface{})
)

// Get returns the session data for the request client.
func Get(name string) (s map[string]interface{}) {
	mu.Lock()
	defer mu.Unlock()
	if s, ok := sessions[name]; ok {
		return s
	}
	return make(map[string]interface{})
}

// Save saves session for the request client.
func Save(key string, s map[string]interface{}) {
	mu.Lock()
	sessions[key] = s
	mu.Unlock()
}
