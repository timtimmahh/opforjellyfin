// cmd/sync.go
package cmd

import (
	"fmt"
	"opforjellyfin/internal/metadata"
	"opforjellyfin/internal/shared"

	"github.com/spf13/cobra"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Update metadata library with new content from GitHub",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := shared.LoadConfig()
		if err != nil {
			return err
		}
		if cmd.Flags().Changed("repo") {
			if err := shared.ValidateMetadataRepo(syncRepo); err != nil {
				return err
			}
			// Saved by the sync once the repo has been fetched successfully.
			cfg.GitHubRepo = syncRepo
		}
		if cfg.TargetDir == "" {
			return fmt.Errorf("no target directory set; use 'setDir' first")
		}
		return metadata.SyncMetadata(cfg)
	},
}

var syncRepo string

func init() {
	syncCmd.Flags().StringVar(&syncRepo, "repo", "", "GitHub metadata repo as owner/name; saved after a successful sync (default "+shared.DefaultMetadataRepo+")")
	rootCmd.AddCommand(syncCmd)
}
