package cmd

import (
	"github.com/spf13/cobra"
)

var dbPath string

var rootCmd = &cobra.Command{
	Use:   "restlens-capture",
	Short: "API traffic capture and analysis tool",
	Long:  "Capture API traffic via a reverse proxy, store locally, and export to REST Lens for analysis.",
}

func init() {
	rootCmd.PersistentFlags().StringVar(&dbPath, "db", "restlens-capture.db", "path to SQLite database file")
}

func Execute() error {
	return rootCmd.Execute()
}
