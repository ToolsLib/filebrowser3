package bolt

import (
	"errors"
	"strings"

	"github.com/asdine/storm/v3"
	"github.com/asdine/storm/v3/q"

	fberrors "github.com/filebrowser/filebrowser/v2/errors"
	"github.com/filebrowser/filebrowser/v2/share"
)

type shareBackend struct {
	db *storm.DB
}

func (s shareBackend) All() ([]*share.Link, error) {
	var v []*share.Link
	err := s.db.All(&v)
	if errors.Is(err, storm.ErrNotFound) {
		return v, fberrors.ErrNotExist
	}

	return v, err
}

func (s shareBackend) FindByUserID(id uint) ([]*share.Link, error) {
	var v []*share.Link
	err := s.db.Select(q.Eq("UserID", id)).Find(&v)
	if errors.Is(err, storm.ErrNotFound) {
		return v, fberrors.ErrNotExist
	}

	return v, err
}

func (s shareBackend) GetByHash(hash string) (*share.Link, error) {
	var v share.Link
	err := s.db.One("Hash", hash, &v)
	if errors.Is(err, storm.ErrNotFound) {
		return nil, fberrors.ErrNotExist
	}

	return &v, err
}

func (s shareBackend) GetPermanent(path string, id uint) (*share.Link, error) {
	var v share.Link
	err := s.db.Select(q.Eq("Path", path), q.Eq("Expire", 0), q.Eq("UserID", id)).First(&v)
	if errors.Is(err, storm.ErrNotFound) {
		return nil, fberrors.ErrNotExist
	}

	return &v, err
}

func (s shareBackend) Gets(path string, id uint) ([]*share.Link, error) {
	var v []*share.Link
	err := s.db.Select(q.Eq("Path", path), q.Eq("UserID", id)).Find(&v)
	if errors.Is(err, storm.ErrNotFound) {
		return v, fberrors.ErrNotExist
	}

	return v, err
}

func (s shareBackend) Save(l *share.Link) error {
	return s.db.Save(l)
}

// Read/modify/write in one transaction: concurrent edits merge independent
// fields, and an edit can never recreate a deleted link.
func (s shareBackend) Update(hash string, change func(*share.Link) error) (*share.Link, error) {
	tx, err := s.db.Begin(true)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	var link share.Link
	if err := tx.One("Hash", hash, &link); err != nil {
		if errors.Is(err, storm.ErrNotFound) {
			return nil, fberrors.ErrNotExist
		}
		return nil, err
	}
	path, owner := link.Path, link.UserID
	if err := change(&link); err != nil {
		return nil, err
	}
	if link.Hash != hash || link.Path != path || link.UserID != owner {
		return nil, fberrors.ErrInvalidDataType
	}
	if err := tx.Save(&link); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &link, nil
}

func (s shareBackend) DeleteExpired(hash string, before int64) (bool, error) {
	tx, err := s.db.Begin(true)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	var link share.Link
	if err := tx.One("Hash", hash, &link); err != nil {
		if errors.Is(err, storm.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	if link.Expire == 0 || link.Expire > before {
		return false, nil
	}
	if err := tx.DeleteStruct(&link); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func (s shareBackend) Delete(hash string) error {
	err := s.db.DeleteStruct(&share.Link{Hash: hash})
	if errors.Is(err, storm.ErrNotFound) {
		return nil
	}
	return err
}

func (s shareBackend) DeleteWithPathPrefix(pathPrefix string, userID uint) error {
	// Share paths are stored without a trailing slash
	prefix := strings.TrimRight(pathPrefix, "/")

	var links []share.Link
	if err := s.db.Prefix("Path", prefix, &links); err != nil {
		if errors.Is(err, storm.ErrNotFound) {
			return nil
		}
		return err
	}

	var err error
	for _, link := range links {
		if link.UserID != userID {
			continue
		}

		if link.Path != prefix && !strings.HasPrefix(link.Path, prefix+"/") {
			continue
		}

		err = errors.Join(err, s.db.DeleteStruct(&share.Link{Hash: link.Hash}))
	}
	return err
}
