package auth

import (
	"errors"
	"testing"
	"time"
)

func TestMemorySessionStoreIDsAreUnique(t *testing.T) {
	store := NewMemorySessionStore(time.Hour)
	defer store.Close()

	first, err := store.Create(&User{Username: "a"})
	if err != nil {
		t.Fatalf("Create() returned unexpected error: %v", err)
	}
	second, err := store.Create(&User{Username: "b"})
	if err != nil {
		t.Fatalf("Create() returned unexpected error: %v", err)
	}

	if first.ID == second.ID {
		t.Error("Create() produced two identical session ids")
	}
}

func TestMemorySessionStoreGetUnknownID(t *testing.T) {
	store := NewMemorySessionStore(time.Hour)
	defer store.Close()

	if _, err := store.Get("does-not-exist"); !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("Get(unknown) error = %v, want ErrSessionNotFound", err)
	}
	if _, err := store.Get(""); !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("Get(\"\") error = %v, want ErrSessionNotFound", err)
	}
}

func TestMemorySessionStoreExpiresSessions(t *testing.T) {
	store := NewMemorySessionStore(time.Nanosecond)
	defer store.Close()

	session, err := store.Create(&User{Username: "gburdell3"})
	if err != nil {
		t.Fatalf("Create() returned unexpected error: %v", err)
	}

	time.Sleep(time.Millisecond)

	if _, err := store.Get(session.ID); !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("Get() on an expired session error = %v, want ErrSessionNotFound", err)
	}
}
