package sessions

import (
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

func TestPersistentAtomicRotationAndRevocation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.db")
	db, err := bolt.Open(path, 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore(db)
	initial := &Session{ID: "id", UserID: 1, Credential: "credential", RefreshHash: "first", Expires: time.Now().Add(time.Hour), AbsoluteExpires: time.Now().Add(2 * time.Hour)}
	if err := store.Create(initial); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = bolt.Open(path, 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	first, second := NewStore(db), NewStore(db)
	if _, err := first.Get("id"); err != nil {
		t.Fatalf("restart lost session: %v", err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, backend := range []*Store{first, second} {
		wg.Add(1)
		go func(st *Store) {
			defer wg.Done()
			_, err := st.Rotate("id", "first", "second", time.Now().Add(4*time.Hour))
			results <- err
		}(backend)
	}
	wg.Wait()
	close(results)
	succeeded, failed := 0, 0
	for err := range results {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, ErrInvalid):
			failed++
		default:
			t.Fatal(err)
		}
	}
	if succeeded != 1 || failed != 1 {
		t.Fatalf("rotation outcomes %d/%d", succeeded, failed)
	}
	if _, err := first.Get("id"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("replayed session remained active: %v", err)
	}
	initial.ID = "fresh"
	if err := first.Create(initial); err != nil {
		t.Fatal(err)
	}
	updated, err := second.Rotate("fresh", "first", "third", time.Now().Add(4*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if !updated.Expires.Equal(initial.AbsoluteExpires) {
		t.Fatal("rotation extended absolute lifetime")
	}
	if err := second.RevokeUser(1); err != nil {
		t.Fatal(err)
	}
	if _, err := first.Get("fresh"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("other store did not see revocation: %v", err)
	}
}

func TestSessionBoundsAndCredentialReplacement(t *testing.T) {
	db, err := bolt.Open(filepath.Join(t.TempDir(), "sessions.db"), 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)
	for i := 0; i < 20; i++ {
		s := &Session{ID: time.Now().Add(time.Duration(i)).Format(time.RFC3339Nano), UserID: 1, Credential: "old", Expires: time.Now().Add(time.Hour), AbsoluteExpires: time.Now().Add(2 * time.Hour)}
		if err := store.Create(s); err != nil {
			t.Fatal(err)
		}
	}
	next := &Session{ID: "new", UserID: 1, Credential: "old", Expires: time.Now().Add(time.Hour), AbsoluteExpires: time.Now().Add(2 * time.Hour)}
	if err := store.Create(next); !errors.Is(err, ErrLimit) {
		t.Fatalf("session bound: %v", err)
	}
	next.Credential = "new"
	if err := store.Create(next); err != nil {
		t.Fatalf("old credentials prevented new login: %v", err)
	}
	next.ID = "expired"
	next.Expires = time.Now().Add(-time.Second)
	if err := store.Create(next); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get("expired"); !errors.Is(err, ErrInvalid) {
		t.Fatal("expired session accepted")
	}
}
