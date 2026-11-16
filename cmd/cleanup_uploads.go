package cmd

import (
	"fmt"
	"time"

	fbhttp "github.com/filebrowser/filebrowser/v2/http"
	"github.com/spf13/cobra"
)

func init() {
	command := &cobra.Command{
		Use:   "cleanup-uploads",
		Short: "Find or remove orphaned upload staging files",
		Long:  "Find orphaned upload journals beneath a root. Dry-run by default. Stop memory-backed servers before applying cleanup; provide the shared Redis URL for a Redis-backed deployment.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			root, _ := cmd.Flags().GetString("root")
			age, _ := cmd.Flags().GetDuration("older-than")
			apply, _ := cmd.Flags().GetBool("apply")
			redisURL, _ := cmd.Flags().GetString("redis")
			if age < time.Hour {
				return fmt.Errorf("older-than must be at least one hour")
			}
			var cache fbhttp.UploadCache
			if redisURL != "" {
				var err error
				cache, err = fbhttp.NewUploadCache(redisURL)
				if err != nil {
					return err
				}
				defer cache.Close()
			}
			found, err := fbhttp.CleanupUploads(root, cache, time.Now().Add(-age), apply)
			for _, name := range found {
				fmt.Fprintln(cmd.OutOrStdout(), name)
			}
			return err
		},
	}
	command.Flags().String("root", ".", "served filesystem root to inspect")
	command.Flags().String("redis", "", "shared upload Redis URL, required when checking a Redis-backed deployment")
	command.Flags().Duration("older-than", 24*time.Hour, "minimum journal age")
	command.Flags().Bool("apply", false, "remove candidates instead of only listing them")
	rootCmd.AddCommand(command)
}
