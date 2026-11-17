package files

import (
	"context"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestListingDefersSubtitleDiscoveryToVideoDetail(t *testing.T) {
	fs := afero.NewMemMapFs()
	for _, name := range []string{"/clips/clip.mp4", "/clips/clip.en.vtt", "/clips/subtitles/clip.srt"} {
		require.NoError(t, afero.WriteFile(fs, name, []byte("fixture"), 0600))
	}
	options := &FileOptions{Fs: fs, Path: "/clips", Expand: true, ReadHeader: true, Checker: performanceAllowAll{}, LeanListing: true}
	directory, err := NewFileInfo(options)
	require.NoError(t, err)
	for _, item := range directory.Items {
		require.Empty(t, item.Subtitles)
	}
	options.Path = "/clips/clip.mp4"
	video, err := NewFileInfo(options)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"/clips/clip.en.vtt", "/clips/subtitles/clip.srt"}, video.Subtitles)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	options.Context = ctx
	_, err = NewFileInfo(options)
	require.ErrorIs(t, err, context.Canceled)
}
