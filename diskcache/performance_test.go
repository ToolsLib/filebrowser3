package diskcache

import (
	"context"
	"fmt"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
	"sync"
	"testing"
)

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
