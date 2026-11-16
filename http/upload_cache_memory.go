package fbhttp

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jellydator/ttlcache/v3"
)

const uploadCacheTTL = 3 * time.Minute

var (
	ErrUploadNotFound = errors.New("upload not found or expired")
	ErrUploadExists   = errors.New("an upload is already active for this destination")
)

// UploadState describes a generation of an upload. Only its private staging
// file may be modified until the entire declared length has arrived.
type UploadState struct {
	ID               string `json:"id"`
	UserID           uint   `json:"userID"`
	Length           int64  `json:"length"`
	Stage            string `json:"stage"`
	Mode             uint32 `json:"mode"`
	Existed          bool   `json:"existed"`
	OriginalSize     int64  `json:"originalSize"`
	OriginalModified int64  `json:"originalModified"`
}

type UploadCache interface {
	Register(key string, state UploadState, remove func() error) error
	Get(key string) (UploadState, error)
	Complete(key, id string) error
	Touch(key, id string) error
	Close()
}

type memoryUploadEntry struct {
	state  UploadState
	remove func() error
}

type memoryUploadCache struct {
	cache *ttlcache.Cache[string, memoryUploadEntry]
	mu    sync.Mutex
}

func newMemoryUploadCache() *memoryUploadCache {
	cache := ttlcache.New[string, memoryUploadEntry]()
	cache.OnEviction(func(_ context.Context, reason ttlcache.EvictionReason, item *ttlcache.Item[string, memoryUploadEntry]) {
		if reason == ttlcache.EvictionReasonExpired && item.Value().remove != nil {
			if err := item.Value().remove(); err != nil {
				fmt.Printf("incomplete upload cleanup: %v\n", err)
			}
		}
	})
	go cache.Start()
	return &memoryUploadCache{cache: cache}
}

func (c *memoryUploadCache) Register(key string, state UploadState, remove func() error) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cache.Get(key) != nil {
		return ErrUploadExists
	}
	c.cache.Set(key, memoryUploadEntry{state: state, remove: remove}, uploadCacheTTL)
	return nil
}

func (c *memoryUploadCache) Get(key string) (UploadState, error) {
	item := c.cache.Get(key)
	if item == nil {
		return UploadState{}, ErrUploadNotFound
	}
	return item.Value().state, nil
}

func (c *memoryUploadCache) Complete(key, id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	state, err := c.Get(key)
	if err != nil {
		return err
	}
	if state.ID != id {
		return ErrUploadNotFound
	}
	c.cache.Delete(key)
	return nil
}

func (c *memoryUploadCache) Touch(key, id string) error {
	state, err := c.Get(key)
	if err != nil {
		return err
	}
	if state.ID != id {
		return ErrUploadNotFound
	}
	return nil // Get refreshes the TTL.
}

func (c *memoryUploadCache) Close() { c.cache.Stop() }

func NewUploadCache(redisURL string) (UploadCache, error) {
	if redisURL != "" {
		return newRedisUploadCache(redisURL)
	}
	return newMemoryUploadCache(), nil
}
