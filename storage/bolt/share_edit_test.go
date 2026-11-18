package bolt

import (
	"errors"
	"sync"
	"testing"
	"time"

	fberrors "github.com/filebrowser/filebrowser/v2/errors"
	"github.com/filebrowser/filebrowser/v2/share"
)

func TestShareUpdatesAreAtomicAndDoNotResurrectDeletedLinks(t *testing.T) {
	back := newTestShareBackend(t)
	if err := back.Save(&share.Link{Hash: "edit", Path: "/file", UserID: 1}); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	wg.Go(func() {
		_, err := back.Update("edit", func(l *share.Link) error { l.Expire = 12345; return nil })
		if err != nil {
			t.Error(err)
		}
	})
	wg.Go(func() {
		_, err := back.Update("edit", func(l *share.Link) error { l.PasswordHash = "new hash"; l.Token = "new token"; return nil })
		if err != nil {
			t.Error(err)
		}
	})
	wg.Wait()
	current, err := back.GetByHash("edit")
	if err != nil || current.Expire != 12345 || current.PasswordHash != "new hash" || current.Token != "new token" {
		t.Fatalf("concurrent field update lost: %+v %v", current, err)
	}
	for _, change := range []func(*share.Link) error{
		func(l *share.Link) error { l.Path = "/other"; return nil },
		func(l *share.Link) error { l.UserID = 2; return nil },
		func(l *share.Link) error { l.Hash = "other"; return nil },
		func(l *share.Link) error { l.Token = "must rollback"; return errors.New("rejected") },
	} {
		if _, err := back.Update("edit", change); err == nil {
			t.Fatal("invalid mutation committed")
		}
	}
	current, _ = back.GetByHash("edit")
	if current.Path != "/file" || current.UserID != 1 || current.Token != "new token" {
		t.Fatal("rejected update was not rolled back")
	}
	if err := back.Delete("edit"); err != nil {
		t.Fatal(err)
	}
	if _, err := back.Update("edit", func(l *share.Link) error { l.Expire = 0; return nil }); !errors.Is(err, fberrors.ErrNotExist) {
		t.Fatal("deleted share recreated")
	}
}

type staleShareSnapshot struct {
	share.StorageBackend
	links []*share.Link
}

type staleHashSnapshot struct {
	share.StorageBackend
	first *share.Link
}

func (s *staleHashSnapshot) GetByHash(hash string) (*share.Link, error) {
	if s.first != nil {
		first := s.first
		s.first = nil
		return first, nil
	}
	return s.StorageBackend.GetByHash(hash)
}

func (s staleShareSnapshot) All() ([]*share.Link, error) { return s.links, nil }

func TestShareExpiryCleanupPreservesExtensionsAndFiltersAdjacentExpiredLinks(t *testing.T) {
	back := newTestShareBackend(t)
	now := time.Now().Unix()
	for _, hash := range []string{"a-expired", "b-expired", "c-expired"} {
		if err := back.Save(&share.Link{Hash: hash, Path: "/file", UserID: 1, Expire: now - 1}); err != nil {
			t.Fatal(err)
		}
	}
	if err := back.Save(&share.Link{Hash: "active", Path: "/file", UserID: 1, Expire: now + 3600}); err != nil {
		t.Fatal(err)
	}
	store := share.NewStorage(back)
	links, err := store.All()
	if err != nil || len(links) != 1 || links[0].Hash != "active" {
		t.Fatalf("expired filtering: %+v %v", links, err)
	}
	// A list can hold an expired snapshot even though another transaction has
	// already extended the record. Cleanup must check the current value.
	stale := &share.Link{Hash: "active", Path: "/file", UserID: 1, Expire: now - 1}
	if deleted, err := back.DeleteExpired("active", now); err != nil || deleted {
		t.Fatal("freshly extended link deleted")
	}
	view := share.NewStorage(staleShareSnapshot{StorageBackend: back, links: []*share.Link{stale}})
	links, err = view.All()
	if err != nil || len(links) != 1 || links[0].Expire != now+3600 {
		t.Fatalf("snapshot cleanup lost extension: %+v %v", links, err)
	}
	racing := share.NewStorage(&staleHashSnapshot{StorageBackend: back, first: stale})
	if link, err := racing.GetByHash("active"); err != nil || link.Expire != now+3600 {
		t.Fatalf("racing expiration read lost link: %+v %v", link, err)
	}
	if _, err := store.Update("active", func(l *share.Link) error { l.Expire = now - 1; return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Update("active", func(l *share.Link) error { l.Expire = 0; return nil }); !errors.Is(err, fberrors.ErrNotExist) {
		t.Fatal("expired link revived")
	}
	if _, err := store.GetByHash("active"); !errors.Is(err, fberrors.ErrNotExist) {
		t.Fatal("expired link served")
	}
}
