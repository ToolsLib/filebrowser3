package fbhttp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type redisUploadCache struct {
	client  *redis.Client
	cleanup *memoryUploadCache
}

func redisContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 5*time.Second)
}

func newRedisUploadCache(redisURL string) (*redisUploadCache, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("invalid redis URL: %w", err)
	}
	client := redis.NewClient(opts)
	ctx, cancel := redisContext()
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("connect to redis: %w", err)
	}
	return &redisUploadCache{client: client, cleanup: newMemoryUploadCache()}, nil
}

func (c *redisUploadCache) filePathKey(key string) string { return "filebrowser:upload:v2:" + key }

func (c *redisUploadCache) Register(key string, state UploadState, remove func() error) error {
	encoded, err := json.Marshal(state)
	if err != nil {
		return err
	}
	ctx, cancel := redisContext()
	defer cancel()
	created, err := c.client.SetNX(ctx, c.filePathKey(key), encoded, uploadCacheTTL).Result()
	if err != nil {
		return err
	}
	if !created {
		return ErrUploadExists
	}
	// A different replica may still be using an upload when our cleanup timer
	// expires. Recheck shared state before removing this generation's files.
	cleanupKey := key + "|" + state.ID
	var cleanup func() error
	cleanup = func() error {
		ctx, cancel := redisContext()
		defer cancel()
		value, err := c.client.Get(ctx, c.filePathKey(key)).Result()
		if err != nil && !errors.Is(err, redis.Nil) {
			c.cleanup.cache.Set(cleanupKey, memoryUploadEntry{state: state, remove: cleanup}, uploadCacheTTL)
			return err
		}
		var current UploadState
		if err == nil && json.Unmarshal([]byte(value), &current) == nil && current.ID == state.ID {
			c.cleanup.cache.Set(cleanupKey, memoryUploadEntry{state: state, remove: cleanup}, uploadCacheTTL)
			return nil
		}
		if remove != nil {
			return remove()
		}
		return nil
	}
	c.cleanup.cache.Set(cleanupKey, memoryUploadEntry{state: state, remove: cleanup}, uploadCacheTTL)
	return nil
}

func (c *redisUploadCache) Get(key string) (UploadState, error) {
	ctx, cancel := redisContext()
	defer cancel()
	value, err := c.client.Get(ctx, c.filePathKey(key)).Bytes()
	if errors.Is(err, redis.Nil) {
		return UploadState{}, ErrUploadNotFound
	}
	if err != nil {
		return UploadState{}, err
	}
	var state UploadState
	if err := json.Unmarshal(value, &state); err != nil {
		return UploadState{}, err
	}
	return state, nil
}

// Compare generations atomically: delayed requests cannot delete or extend a
// replacement upload's state.
const uploadGenerationScript = `
local value = redis.call('GET', KEYS[1])
if not value then return 0 end
local state = cjson.decode(value)
if state.id ~= ARGV[1] then return 0 end
if ARGV[2] == 'delete' then return redis.call('DEL', KEYS[1]) end
return redis.call('PEXPIRE', KEYS[1], ARGV[2])
`

func (c *redisUploadCache) update(key, id, action string) error {
	ctx, cancel := redisContext()
	defer cancel()
	count, err := c.client.Eval(ctx, uploadGenerationScript, []string{c.filePathKey(key)}, id, action).Int()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrUploadNotFound
	}
	return nil
}

func (c *redisUploadCache) Complete(key, id string) error { return c.update(key, id, "delete") }
func (c *redisUploadCache) Touch(key, id string) error {
	return c.update(key, id, fmt.Sprint(uploadCacheTTL.Milliseconds()))
}
func (c *redisUploadCache) Close() { c.cleanup.Close(); _ = c.client.Close() }
