package cmd

import (
	"fmt"

	"github.com/Edthing/restlens-capture/internal/export"
	"github.com/Edthing/restlens-capture/internal/report"
	"github.com/Edthing/restlens-capture/internal/storage"
	"github.com/spf13/cobra"
)

var reportFormat string

var reportCmd = &cobra.Command{
	Use:   "report",
	Short: "Generate a traffic summary from captured data",
	Long:  "Reads captured exchanges and produces a summary of discovered endpoints, hit counts, and latency stats.",
	RunE: func(cmd *cobra.Command, args []string) error {
		db, err := storage.OpenDB(dbPath)
		if err != nil {
			return fmt.Errorf("failed to open database: %w", err)
		}
		defer db.Close()

		exchanges, err := storage.LoadAllExchanges(db)
		if err != nil {
			return fmt.Errorf("failed to load exchanges: %w", err)
		}

		if len(exchanges) == 0 {
			fmt.Println("No captured traffic found. Run `restlens-capture proxy` first.")
			return nil
		}

		summary := report.BuildSummary(exchanges, export.GroupPatterns(exchanges))

		switch reportFormat {
		case "json":
			return report.RenderJSON(summary, cmd.OutOrStdout())
		default:
			report.RenderTerminal(summary, cmd.OutOrStdout())
			return nil
		}
	},
}

func init() {
	reportCmd.Flags().StringVar(&reportFormat, "format", "terminal", "output format: terminal or json")
	rootCmd.AddCommand(reportCmd)
}
