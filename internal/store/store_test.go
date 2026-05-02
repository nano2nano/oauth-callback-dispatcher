package store

import (
	"testing"
	"time"
)

func TestRegisterConsumeAndReplay(t *testing.T) {
	now := time.Date(2026, 5, 3, 0, 0, 0, 0, time.UTC)
	s := New(10*time.Minute, func() time.Time { return now })
	if err := s.Register("state", "https://feat.myapp.localhost"); err != nil {
		t.Fatal("expected register success")
	}
	if err := s.Register("state", "https://other.myapp.localhost"); err != ErrDuplicateState {
		t.Fatal("expected duplicate register to fail")
	}
	entry, ok := s.Consume("state")
	if !ok || entry.Origin != "https://feat.myapp.localhost" {
		t.Fatalf("unexpected consume result: %#v %v", entry, ok)
	}
	if _, ok := s.Consume("state"); ok {
		t.Fatal("expected replay to fail")
	}
}

func TestExpiredStateFails(t *testing.T) {
	now := time.Date(2026, 5, 3, 0, 0, 0, 0, time.UTC)
	s := New(time.Second, func() time.Time { return now })
	if err := s.Register("state", "https://feat.myapp.localhost"); err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Second)
	if _, ok := s.Consume("state"); ok {
		t.Fatal("expected expired state to fail")
	}
}

func TestRegisterFailsWhenFull(t *testing.T) {
	s := New(10*time.Minute, time.Now)
	s.SetMaxEntries(1)
	if err := s.Register("state1", "https://feat.myapp.localhost"); err != nil {
		t.Fatal(err)
	}
	if err := s.Register("state2", "https://feat.myapp.localhost"); err != ErrStoreFull {
		t.Fatalf("err = %v, want %v", err, ErrStoreFull)
	}
}
