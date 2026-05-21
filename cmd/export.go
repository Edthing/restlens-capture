package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/Edthing/restlens-capture/internal/export"
	"github.com/Edthing/restlens-capture/internal/storage"
	"github.com/spf13/cobra"
)

var (
	exportOpenAPI    bool
	exportToRestLens bool
	exportOutput     string
	exportProject    string
	exportToken      string
	exportServer     string
	exportTag        string
)

// parseProject splits an "<org>/<project>" identifier into its parts.
func parseProject(s string) (org, project string, err error) {
	parts := strings.SplitN(s, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("project must be in the form <org>/<project> (got %q)", s)
	}
	return parts[0], parts[1], nil
}

// firstNonEmpty returns the first non-empty string from the supplied values.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

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
			spec := export.GenerateOpenAPI(exchanges)

			org, project, err := parseProject(firstNonEmpty(exportProject, os.Getenv("RESTLENS_PROJECT")))
			if err != nil {
				return err
			}

			result, err := export.UploadToRestLens(cmd.Context(), spec, export.UploadOptions{
				BaseURL:     firstNonEmpty(exportServer, os.Getenv("RESTLENS_URL"), os.Getenv("RESTLENS_SERVER")),
				Token:       firstNonEmpty(exportToken, os.Getenv("RESTLENS_TOKEN")),
				OrgSlug:     org,
				ProjectSlug: project,
				Tag:         exportTag,
			})
			if err != nil {
				return fmt.Errorf("REST Lens export failed: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(),
				"Uploaded API profile to REST Lens: spec %s (version %d) — evaluation status: %s\n",
				result.Specification.ID, result.Specification.Version, result.Evaluation.Status)
		}

		return nil
	},
}

func init() {
	exportCmd.Flags().BoolVar(&exportOpenAPI, "openapi", false, "generate an OpenAPI spec from captured traffic")
	exportCmd.Flags().BoolVar(&exportToRestLens, "to-restlens", false, "upload the inferred API profile to REST Lens")
	exportCmd.Flags().StringVarP(&exportOutput, "output", "o", "", "output file path (default: stdout)")
	exportCmd.Flags().StringVar(&exportProject, "project", "", "REST Lens project as <org>/<project> (for --to-restlens; or RESTLENS_PROJECT)")
	exportCmd.Flags().StringVar(&exportToken, "token", "", "REST Lens project API token (for --to-restlens; or RESTLENS_TOKEN)")
	exportCmd.Flags().StringVar(&exportServer, "server", "", "REST Lens base URL (default https://restlens.com; or RESTLENS_URL/RESTLENS_SERVER)")
	exportCmd.Flags().StringVar(&exportTag, "tag", "", "optional version tag for the uploaded spec")
	rootCmd.AddCommand(exportCmd)
}
