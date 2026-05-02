package store

import (
	"errors"
	"sync"
	"time"
)

var (
	ErrDuplicateState = errors.New("state already registered")
	ErrStoreFull      = errors.New("state store is full")
)

type Clock func() time.Time

type Entry struct {
	Origin    string
	ExpiresAt time.Time
}

type Store struct {
	mu    sync.Mutex
	ttl   time.Duration
	now   Clock
	limit int
	items map[string]Entry
}

func New(ttl time.Duration, now Clock) *Store {
	if now == nil {
		now = time.Now
	}
	return &Store{ttl: ttl, now: now, items: make(map[string]Entry)}
}

func (s *Store) SetMaxEntries(limit int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.limit = limit
}

func (s *Store) Register(state, origin string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deleteExpiredLocked(s.now())
	if _, ok := s.items[state]; ok {
		return ErrDuplicateState
	}
	if s.limit > 0 && len(s.items) >= s.limit {
		return ErrStoreFull
	}
	s.items[state] = Entry{Origin: origin, ExpiresAt: s.now().Add(s.ttl)}
	return nil
}

func (s *Store) Consume(state string) (Entry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	entry, ok := s.items[state]
	if !ok {
		return Entry{}, false
	}
	delete(s.items, state)
	if !entry.ExpiresAt.After(now) {
		return Entry{}, false
	}
	return entry, true
}

func (s *Store) Cleanup() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.deleteExpiredLocked(s.now())
}

func (s *Store) deleteExpiredLocked(now time.Time) int {
	deleted := 0
	for state, entry := range s.items {
		if !entry.ExpiresAt.After(now) {
			delete(s.items, state)
			deleted++
		}
	}
	return deleted
}
