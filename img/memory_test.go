package img

import (
	"bytes"
	"context"
	"encoding/binary"
	"github.com/stretchr/testify/require"
	"io"
	"testing"
	"time"
)

func TestDecodedPixelBudgetBeforeAllocation(t *testing.T) {
	// A valid BMP header alone: no giant pixel allocation in this test.
	header := make([]byte, 54)
	copy(header, "BM")
	binary.LittleEndian.PutUint32(header[10:], 54)
	binary.LittleEndian.PutUint32(header[14:], 40)
	binary.LittleEndian.PutUint32(header[18:], 6000)
	binary.LittleEndian.PutUint32(header[22:], 6000)
	binary.LittleEndian.PutUint16(header[26:], 1)
	binary.LittleEndian.PutUint16(header[28:], 24)
	require.ErrorIs(t, New(4).Resize(context.Background(), bytes.NewReader(header), 256, 256, io.Discard), ErrImageTooLarge)
}

func TestQueuedMemoryAdmissionCanBeCanceled(t *testing.T) {
	service := New(2)
	require.NoError(t, service.memory.Acquire(context.Background(), imageMemoryMiB))
	defer service.memory.Release(imageMemoryMiB)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	input := newGrayPng(t, 20, 20)
	defer input.Close()
	require.ErrorIs(t, service.Resize(ctx, input, 10, 10, io.Discard), context.DeadlineExceeded)
}
