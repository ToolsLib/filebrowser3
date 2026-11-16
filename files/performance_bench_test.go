package files

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"
)

// These benchmarks use generated fixtures and never read the running instance.
type performanceAllowAll struct{}

func (performanceAllowAll) Check(string) bool { return true }

type performanceResolvedChecker struct{ fs afero.Fs }

func (c performanceResolvedChecker) Check(name string) bool {
	_, err := ResolveVirtualPath(c.fs, name)
	return err == nil
}

func BenchmarkPerformanceListing(b *testing.B) {
	for _, kind := range []string{"text", "video", "video-subs"} {
		for _, count := range []int{100, 1000} {
			b.Run(fmt.Sprintf("%s/%d", kind, count), func(b *testing.B) {
				fs := afero.NewMemMapFs()
				ext := ".txt"
				if kind != "text" {
					ext = ".mp4"
				}
				for n := 0; n < count; n++ {
					if err := afero.WriteFile(fs, fmt.Sprintf("/clip-%05d%s", n, ext), []byte("fixture"), 0600); err != nil {
						b.Fatal(err)
					}
				}
				if kind == "video-subs" {
					if err := fs.Mkdir("/subs", 0700); err != nil {
						b.Fatal(err)
					}
					for n := 0; n < count; n++ {
						if err := afero.WriteFile(fs, fmt.Sprintf("/subs/clip-%05d.srt", n), []byte("subtitle"), 0600); err != nil {
							b.Fatal(err)
						}
					}
				}
				opts := &FileOptions{Fs: fs, Path: "/", Expand: true, ReadHeader: true, Checker: performanceAllowAll{}}
				b.ReportAllocs()
				b.ResetTimer()
				for b.Loop() {
					info, err := NewFileInfo(opts)
					if err != nil {
						b.Fatal(err)
					}
					if info.NumFiles != count {
						b.Fatalf("got %d files", info.NumFiles)
					}
				}
			})
		}
	}
}

func BenchmarkPerformanceNativeListing(b *testing.B) {
	for _, count := range []int{100, 1000} {
		root := b.TempDir()
		backing := afero.NewOsFs()
		for n := 0; n < count; n++ {
			if err := afero.WriteFile(backing, filepath.Join(root, fmt.Sprintf("item-%05d.txt", n)), []byte("fixture"), 0600); err != nil {
				b.Fatal(err)
			}
		}
		for _, resolveChecks := range []bool{false, true} {
			b.Run(fmt.Sprintf("files=%d/resolve-checks=%t", count, resolveChecks), func(b *testing.B) {
				fs := NewScopedFs(backing, root)
				var checker interface{ Check(string) bool } = performanceAllowAll{}
				if resolveChecks {
					checker = performanceResolvedChecker{fs}
				}
				opts := &FileOptions{Fs: fs, Path: "/", Expand: true, ReadHeader: true, Checker: checker}
				b.ReportAllocs()
				for b.Loop() {
					if _, err := NewFileInfo(opts); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
