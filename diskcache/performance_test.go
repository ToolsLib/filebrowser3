package diskcache

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
)

func TestOpenReaderSurvivesReplacementAndDeletion(t *testing.T) {
	for _, name := range []string{"short", "long-unicode"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			if name == "long-unicode" {
				root = filepath.Join(root, strings.Repeat("a", 100), strings.Repeat("b", 100), "préviews")
			}
			checkOpenReaderSurvivesReplacementAndDeletion(t, root)
		})
	}
}

func checkOpenReaderSurvivesReplacementAndDeletion(t *testing.T, root string) {
	t.Helper()
	ctx := context.Background()
	cache := New(afero.NewOsFs(), root)
	require.NoError(t, cache.Store(ctx, "key", []byte("old preview")))
	old, found, err := cache.Open(ctx, "key")
	require.NoError(t, err)
	require.True(t, found)
	t.Cleanup(func() { _ = old.Close() })

	require.NoError(t, cache.Store(ctx, "key", []byte("new preview")))
	value, err := io.ReadAll(old)
	require.NoError(t, err)
	require.Equal(t, "old preview", string(value))

	current, found, err := cache.Open(ctx, "key")
	require.NoError(t, err)
	require.True(t, found)
	t.Cleanup(func() { _ = current.Close() })
	require.NoError(t, cache.Delete(ctx, "key"))
	value, err = io.ReadAll(current)
	require.NoError(t, err)
	require.Equal(t, "new preview", string(value))
	_, found, err = cache.Load(ctx, "key")
	require.NoError(t, err)
	require.False(t, found)
}

func TestQuotaRestartAndAtomicReaders(t *testing.T) {
	fs := afero.NewOsFs()
	root := t.TempDir()
	ctx := context.Background()
	cache := NewWithLimit(fs, root, 12)
	require.NoError(t, cache.Store(ctx, "one", []byte("111111")))
	require.NoError(t, cache.Store(ctx, "two", []byte("222222")))
	require.NoError(t, cache.Store(ctx, "three", []byte("333333")))
	require.LessOrEqual(t, cache.bytes, int64(12))
	require.LessOrEqual(t, len(cache.entries), 2)
	cache = NewWithLimit(fs, root, 6)
	_, _, err := cache.Load(ctx, "three")
	require.NoError(t, err)
	require.LessOrEqual(t, cache.bytes, int64(6))
	require.LessOrEqual(t, len(cache.entries), 1)
	cache = NewWithLimit(fs, root, 1024)
	var wg sync.WaitGroup
	for n := 0; n < 12; n++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			value := []byte(fmt.Sprintf("value-%04d", n))
			require.NoError(t, cache.Store(ctx, "same", value))
			body, ok, err := cache.Load(ctx, "same")
			require.NoError(t, err)
			require.True(t, ok)
			require.Len(t, body, len(value))
		}(n)
	}
	wg.Wait()
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	require.Error(t, cache.Store(ctx, "no", []byte("no")))
}
