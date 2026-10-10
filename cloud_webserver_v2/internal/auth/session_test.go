package auth

import (
	"errors"
	"testing"
	"time"
)

func TestMemorySessionStoreCreateAndGet(t *testing.T) {
	store := NewMemorySessionStore(time.Hour)
	defer store.Close()

	user := &User{Username: "gburdell3"}

	session, err := store.Create(user)
	if err != nil {
		t.Fatalf("Create() returned unexpected error: %v", err)
	}
	if session.ID == "" {
		t.Fatal("Create() returned a session without an id")
	}
	if session.User != user {
		t.Error("Create() did not keep the authenticated user on the session")
	}

	stored, err := store.Get(session.ID)
	if err != nil {
		t.Fatalf("Get() returned unexpected error: %v", err)
	}
	if stored.User.Username != "gburdell3" {
		t.Errorf("stored user = %q, want %q", stored.User.Username, "gburdell3")
	}
}

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

func TestMemorySessionStoreDelete(t *testing.T) {
	store := NewMemorySessionStore(time.Hour)
	defer store.Close()

	session, err := store.Create(&User{Username: "gburdell3"})
	if err != nil {
		t.Fatalf("Create() returned unexpected error: %v", err)
	}

	store.Delete(session.ID)

	if _, err := store.Get(session.ID); !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("Get() after Delete() error = %v, want ErrSessionNotFound", err)
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
