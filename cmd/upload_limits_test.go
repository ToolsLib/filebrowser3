package cmd

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/asdine/storm/v3"
	"github.com/filebrowser/filebrowser/v2/settings"
	"github.com/filebrowser/filebrowser/v2/storage/bolt"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

func TestUploadCeilingConfigurationAndRC1Migration(t *testing.T) {
	for _, tc := range []struct {
		name   string
		saved  int64
		env    string
		flag   string
		config bool
		want   int64
		source string
	}{
		{"new default", 0, "", "", false, 0, ""},
		{"RC1 default", 10 << 30, "", "", false, 0, ""},
		{"stored nondefault ceiling", 100 << 30, "", "", false, 100 << 30, "configuration"},
		{"environment ceiling", 10 << 30, "10737418240", "", false, 10 << 30, "environment"},
		{"flag takes precedence", 0, "10737418240", "5368709120", false, 5 << 30, "configuration"},
		{"explicit config default", 10 << 30, "", "", true, 10 << 30, "configuration"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("FB_MAX_UPLOAD_SIZE", tc.env)
			db, err := storm.Open(filepath.Join(t.TempDir(), "settings.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			st, err := bolt.NewStorage(db)
			if err != nil {
				t.Fatal(err)
			}
			if err := st.Settings.SaveServer(&settings.Server{MaxUploadSize: tc.saved}); err != nil {
				t.Fatal(err)
			}
			flags := pflag.NewFlagSet("upload-policy", pflag.ContinueOnError)
			addServerFlags(flags)
			if tc.flag != "" {
				if err := flags.Set("maxUploadSize", tc.flag); err != nil {
					t.Fatal(err)
				}
			}
			v := viper.New()
			v.SetEnvPrefix("FB")
			v.AutomaticEnv()
			v.SetEnvKeyReplacer(strings.NewReplacer("MAXUPLOADSIZE", "MAX_UPLOAD_SIZE"))
			if err := v.BindPFlags(flags); err != nil {
				t.Fatal(err)
			}
			if tc.config {
				v.Set("maxUploadSize", int64(10)<<30)
			}
			server, err := getServerSettings(v, st, flags)
			if err != nil {
				t.Fatal(err)
			}
			if server.MaxUploadSize != tc.want || server.MaxUploadSizeSource != tc.source {
				t.Fatalf("ceiling=%d source=%q, want %d %q", server.MaxUploadSize, server.MaxUploadSizeSource, tc.want, tc.source)
			}
		})
	}
}
