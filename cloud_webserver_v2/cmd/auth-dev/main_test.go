package main

import (
	"context"
	"testing"
)

func TestMemoryUsersRejectDuplicateSignup(t *testing.T) {
	users := &memoryUsers{users: map[string]*memoryUser{}}
	if err := users.Create(context.Background(), "person@example.com", []byte("hash")); err != nil {
		t.Fatal(err)
	}
	if err := users.Create(context.Background(), "person@example.com", []byte("other")); err == nil {
		t.Fatal("duplicate signup succeeded")
	}
}
