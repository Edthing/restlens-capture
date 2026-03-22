package cmd

import (
	"fmt"
	"os"

	"github.com/Edthing/restlens-capture/internal/export"
	"github.com/Edthing/restlens-capture/internal/storage"
	"github.com/spf13/cobra"
)

var (
	exportOpenAPI   bool
	exportToRestLens bool
	exportOutput    string
)

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export captured data as OpenAPI spec or to REST Lens",
	RunE: func(cmd *cobra.Command, args []string) error {
		if !exportOpenAPI && !exportToRestLens {
			return fmt.Errorf("specify --openapi or --to-restlens")
		}

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

		if exportOpenAPI {
			spec := export.GenerateOpenAPI(exchanges)
			out := cmd.OutOrStdout()
			if exportOutput != "" {
				f, err := os.Create(exportOutput)
				if err != nil {
					return fmt.Errorf("failed to create output file: %w", err)
				}
				defer f.Close()
				out = f
			}
			return export.WriteOpenAPIYAML(spec, out)
		}

		if exportToRestLens {
			fmt.Println("REST Lens export is coming soon. Stay tuned!")
		}

		return nil
	},
}

func init() {
	exportCmd.Flags().BoolVar(&exportOpenAPI, "openapi", false, "generate an OpenAPI spec from captured traffic")
	exportCmd.Flags().BoolVar(&exportToRestLens, "to-restlens", false, "export API profile to REST Lens (coming soon)")
	exportCmd.Flags().StringVarP(&exportOutput, "output", "o", "", "output file path (default: stdout)")
	rootCmd.AddCommand(exportCmd)
}
