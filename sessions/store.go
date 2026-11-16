// Package sessions provides durable, bounded session state in the application's
// database. Refresh rotation and revocation use database write transactions.
package sessions

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"time"

	bolt "go.etcd.io/bbolt"
)

var (
	ErrInvalid = errors.New("invalid or expired session")
	ErrLimit   = errors.New("session limit reached")
	bucket     = []byte("sessions-v1")
)

const AbsoluteLifetime = 24 * time.Hour

type Session struct {
	ID              string
	UserID          uint
	Credential      string
	RefreshHash     string
	Expires         time.Time
	AbsoluteExpires time.Time
}

func (s *Session) Active(now time.Time) bool {
	return s.Expires.After(now) && s.AbsoluteExpires.After(now)
}

type Store struct{ db *bolt.DB }

func NewStore(db *bolt.DB) *Store { return &Store{db: db} }

func (st *Store) Create(session *Session) error {
	return st.db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists(bucket)
		if err != nil {
			return err
		}
		count, userCount := 0, 0
		cursor := b.Cursor()
		for key, value := cursor.First(); key != nil; key, value = cursor.Next() {
			var old Session
			if err := json.Unmarshal(value, &old); err != nil {
				return err
			}
			if !old.Active(time.Now()) || (old.UserID == session.UserID && old.Credential != session.Credential) {
				if err := cursor.Delete(); err != nil {
					return err
				}
				continue
			}
			count++
			if old.UserID == session.UserID {
				userCount++
			}
		}
		if count >= 10000 || userCount >= 20 {
			return ErrLimit
		}
		if b.Get([]byte(session.ID)) != nil {
			return ErrInvalid
		}
		data, err := json.Marshal(session)
		if err != nil {
			return err
		}
		return b.Put([]byte(session.ID), data)
	})
}

func (st *Store) Get(id string) (*Session, error) {
	var session Session
	err := st.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucket)
		if b == nil {
			return ErrInvalid
		}
		value := b.Get([]byte(id))
		if value == nil {
			return ErrInvalid
		}
		return json.Unmarshal(value, &session)
	})
	if err != nil {
		return nil, err
	}
	if !session.Active(time.Now()) {
		return nil, ErrInvalid
	}
	return &session, nil
}

// Rotate consumes the refresh secret exactly once. Reuse revokes the session,
// including its previously issued access tokens. Callers serialize browser tabs.
func (st *Store) Rotate(id, previous, next string, expires time.Time) (*Session, error) {
	var session Session
	replayed := false
	err := st.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucket)
		if b == nil {
			return ErrInvalid
		}
		value := b.Get([]byte(id))
		if value == nil {
			return ErrInvalid
		}
		if err := json.Unmarshal(value, &session); err != nil {
			return err
		}
		if !session.Active(time.Now()) {
			return ErrInvalid
		}
		if subtle.ConstantTimeCompare([]byte(previous), []byte(session.RefreshHash)) != 1 {
			replayed = true
			return b.Delete([]byte(id))
		}
		session.RefreshHash = next
		session.Expires = expires
		if expires.After(session.AbsoluteExpires) {
			session.Expires = session.AbsoluteExpires
		}
		data, err := json.Marshal(&session)
		if err != nil {
			return err
		}
		return b.Put([]byte(id), data)
	})
	if err != nil {
		return nil, err
	}
	if replayed {
		return nil, ErrInvalid
	}
	return &session, nil
}

func (st *Store) Revoke(id string) error {
	return st.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucket)
		if b == nil {
			return nil
		}
		return b.Delete([]byte(id))
	})
}

func (st *Store) RevokeUser(id uint) error {
	return st.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucket)
		if b == nil {
			return nil
		}
		cursor := b.Cursor()
		for key, value := cursor.First(); key != nil; key, value = cursor.Next() {
			var session Session
			if err := json.Unmarshal(value, &session); err != nil {
				return err
			}
			if session.UserID == id {
				if err := cursor.Delete(); err != nil {
					return err
				}
			}
		}
		return nil
	})
}
