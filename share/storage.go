package share

import (
	"errors"
	"time"

	fberrors "github.com/filebrowser/filebrowser/v2/errors"
)

// StorageBackend is the interface to implement for a share storage.
type StorageBackend interface {
	All() ([]*Link, error)
	FindByUserID(id uint) ([]*Link, error)
	GetByHash(hash string) (*Link, error)
	GetPermanent(path string, id uint) (*Link, error)
	Gets(path string, id uint) ([]*Link, error)
	Save(s *Link) error
	Update(hash string, change func(*Link) error) (*Link, error)
	DeleteExpired(hash string, before int64) (bool, error)
	Delete(hash string) error
	DeleteWithPathPrefix(path string, userID uint) error
}

// Storage is a storage.
type Storage struct {
	back StorageBackend
}

// NewStorage creates a share links storage from a backend.
func NewStorage(back StorageBackend) *Storage {
	return &Storage{back: back}
}

// All wraps a StorageBackend.All.
func (s *Storage) All() ([]*Link, error) {
	links, err := s.back.All()

	if err != nil {
		return nil, err
	}

	return s.active(links)
}

// FindByUserID wraps a StorageBackend.FindByUserID.
func (s *Storage) FindByUserID(id uint) ([]*Link, error) {
	links, err := s.back.FindByUserID(id)

	if err != nil {
		return nil, err
	}

	return s.active(links)
}

// GetByHash wraps a StorageBackend.GetByHash.
func (s *Storage) GetByHash(hash string) (*Link, error) {
	for attempt := 0; attempt < 3; attempt++ {
		link, err := s.back.GetByHash(hash)
		if err != nil {
			return nil, err
		}
		now := time.Now().Unix()
		if link.Expire == 0 || link.Expire > now {
			return link, nil
		}
		deleted, err := s.back.DeleteExpired(hash, now)
		if err != nil {
			return nil, err
		}
		if deleted {
			return nil, fberrors.ErrNotExist
		}
		// A concurrent edit may have extended the old snapshot. Read it again.
	}
	return nil, fberrors.ErrNotExist
}

// GetPermanent wraps a StorageBackend.GetPermanent
func (s *Storage) GetPermanent(path string, id uint) (*Link, error) {
	return s.back.GetPermanent(path, id)
}

// Gets wraps a StorageBackend.Gets
func (s *Storage) Gets(path string, id uint) ([]*Link, error) {
	links, err := s.back.Gets(path, id)

	if err != nil {
		return nil, err
	}

	return s.active(links)
}

func (s *Storage) active(links []*Link) ([]*Link, error) {
	active := make([]*Link, 0, len(links))
	now := time.Now().Unix()
	for _, link := range links {
		if link.Expire != 0 && link.Expire <= now {
			current, err := s.GetByHash(link.Hash)
			if errors.Is(err, fberrors.ErrNotExist) {
				continue
			}
			if err != nil {
				return nil, err
			}
			link = current
		}
		active = append(active, link)
	}
	return active, nil
}

func (s *Storage) Update(hash string, change func(*Link) error) (*Link, error) {
	return s.back.Update(hash, func(link *Link) error {
		if link.Expire != 0 && link.Expire <= time.Now().Unix() {
			return fberrors.ErrNotExist
		}
		return change(link)
	})
}

// Save wraps a StorageBackend.Save
func (s *Storage) Save(l *Link) error {
	return s.back.Save(l)
}

// Delete wraps a StorageBackend.Delete
func (s *Storage) Delete(hash string) error {
	return s.back.Delete(hash)
}

func (s *Storage) DeleteWithPathPrefix(path string, userID uint) error {
	return s.back.DeleteWithPathPrefix(path, userID)
}
