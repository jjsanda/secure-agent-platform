package main

import (
	"sync"
	"time"
)

// authCode is the server-side state bound to a single-use authorization code
// while the client completes the PKCE exchange at the token endpoint.
type authCode struct {
	subject       string
	codeChallenge string // S256 challenge the code_verifier is checked against
	redirectURI   string
	nonce         string
	clientID      string
	scope         string
	expiry        time.Time
}

// refreshRecord is the server-side state behind an opaque refresh token.
type refreshRecord struct {
	subject string
	scope   string
	nonce   string
	expiry  time.Time
}

// memStore is a tiny concurrency-safe map keyed by an opaque token. Values are
// removed on consume, which gives single-use authorization codes and rotating
// refresh tokens for free.
//
// State is in-memory only: restarting the provider invalidates every code and
// refresh token, which is exactly the behaviour wanted from a zero-dependency
// dev IdP. Expiry is enforced by callers when a value is consumed.
type memStore[V any] struct {
	mu sync.Mutex
	m  map[string]V
}

func newMemStore[V any]() *memStore[V] {
	return &memStore[V]{m: make(map[string]V)}
}

// put stores v under key, overwriting any existing entry.
func (s *memStore[V]) put(key string, v V) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[key] = v
}

// consume atomically returns and removes the value, so a code or refresh token
// can be redeemed at most once.
func (s *memStore[V]) consume(key string) (V, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.m[key]
	if ok {
		delete(s.m, key)
	}
	return v, ok
}

// delete removes key if present (best-effort revocation).
func (s *memStore[V]) delete(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, key)
}
