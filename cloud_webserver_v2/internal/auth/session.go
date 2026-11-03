package auth

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"sync"
	"time"
)

// ErrSessionNotFound is returned when a session id is unknown, expired, or was deleted.
var ErrSessionNotFound = errors.New("session not found")

const sessionIDBytes = 32

// Session associates our own opaque session id with the user that owns it.
// The id is the only thing stored in the browser cookie; the user never leaves the server.
type Session struct {
	ID        string
	User      *User
	CreatedAt time.Time
	ExpiresAt time.Time
}

// SessionStore keeps track of active sessions.
type SessionStore interface {
	// Create starts a new session for an authenticated user.
	Create(user *User) (*Session, error)
	// Get returns an active session, or ErrSessionNotFound if it does not exist or expired.
	Get(id string) (*Session, error)
	// Delete removes a session, for example on logout.
	Delete(id string)
}

// MemorySessionStore keeps sessions in memory. It is enough for a single server
// instance; if the server is ever scaled horizontally or sessions have to survive
// restarts, this can be swapped for a database backed SessionStore.
type MemorySessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	ttl      time.Duration
	stop     chan struct{}
	stopOnce sync.Once
}

func NewMemorySessionStore(ttl time.Duration) *MemorySessionStore {
	store := &MemorySessionStore{
		sessions: make(map[string]*Session),
		ttl:      ttl,
		stop:     make(chan struct{}),
	}

	go store.reapExpiredLoop()
	return store
}

func (s *MemorySessionStore) Create(user *User) (*Session, error) {
	id, err := newSessionID()
	if err != nil {
		return nil, err
	}

	now := time.Now()
	session := &Session{
		ID:        id,
		User:      user,
		CreatedAt: now,
		ExpiresAt: now.Add(s.ttl),
	}

	s.mu.Lock()
	s.sessions[id] = session
	s.mu.Unlock()

	return session, nil
}

func (s *MemorySessionStore) Get(id string) (*Session, error) {
	if id == "" {
		return nil, ErrSessionNotFound
	}

	s.mu.RLock()
	session, ok := s.sessions[id]
	s.mu.RUnlock()

	if !ok {
		return nil, ErrSessionNotFound
	}

	if time.Now().After(session.ExpiresAt) {
		s.Delete(id)
		return nil, ErrSessionNotFound
	}

	return session, nil
}

func (s *MemorySessionStore) Delete(id string) {
	s.mu.Lock()
	delete(s.sessions, id)
	s.mu.Unlock()
}

// Close stops the background cleanup goroutine.
func (s *MemorySessionStore) Close() {
	s.stopOnce.Do(func() {
		close(s.stop)
	})
}

// reapExpiredLoop periodically drops sessions that have expired so they don't pile up
// when no new logins happen for a while.
func (s *MemorySessionStore) reapExpiredLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-s.stop:
			return
		case <-ticker.C:
			s.reapExpired()
		}
	}
}

func (s *MemorySessionStore) reapExpired() {
	now := time.Now()

	s.mu.Lock()
	for id, session := range s.sessions {
		if now.After(session.ExpiresAt) {
			delete(s.sessions, id)
		}
	}
	s.mu.Unlock()
}

func newSessionID() (string, error) {
	bytes := make([]byte, sessionIDBytes)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}
