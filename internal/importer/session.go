// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

package importer

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// sessionMaxAge bounds how long a parsed-but-unconfirmed import waits
// before it's swept — someone who uploads a file, looks at the preview,
// and never clicks confirm shouldn't hold memory forever.
const sessionMaxAge = 30 * time.Minute

// Session is one parsed-but-not-yet-executed import, held between the
// preview step (which produces it) and the confirm step (which consumes
// it exactly once).
type Session struct {
	ConnName   string
	EntityType string
	Rows       []Row
	createdAt  time.Time
}

// SessionStore holds parsed import sessions in memory only — nothing
// here is persisted, and nothing needs to be: a session that's lost on
// restart just means re-uploading the file, not a data-loss event of
// any consequence, since nothing has been written to xolu yet.
type SessionStore struct {
	mu       sync.Mutex
	sessions map[string]Session
}

// NewSessionStore returns an empty store.
func NewSessionStore() *SessionStore {
	return &SessionStore{sessions: make(map[string]Session)}
}

// Put stores sess and returns a random ID to retrieve it by. Sweeps
// expired sessions first — lazy, on-access cleanup rather than a
// background goroutine, which is all a single-operator local tool needs
// and avoids any lifecycle management for a ticker.
func (s *SessionStore) Put(sess Session) (string, error) {
	id, err := randomID()
	if err != nil {
		return "", fmt.Errorf("generating session id: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.sweepLocked()
	sess.createdAt = time.Now()
	s.sessions[id] = sess
	return id, nil
}

// Take retrieves and removes the session for id — single-use, since a
// confirmed import should not be replayable by revisiting the same URL.
// ok is false if id is unknown or has expired.
func (s *SessionStore) Take(id string) (Session, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.sweepLocked()
	sess, ok := s.sessions[id]
	if !ok {
		return Session{}, false
	}
	delete(s.sessions, id)
	return sess, true
}

// sweepLocked removes expired sessions. Caller must hold s.mu.
func (s *SessionStore) sweepLocked() {
	cutoff := time.Now().Add(-sessionMaxAge)
	for id, sess := range s.sessions {
		if sess.createdAt.Before(cutoff) {
			delete(s.sessions, id)
		}
	}
}

func randomID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
