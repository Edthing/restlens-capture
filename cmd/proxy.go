package cmd

import (
	"fmt"
	"log"

	"github.com/Edthing/restlens-capture/internal/proxy"
	"github.com/Edthing/restlens-capture/internal/storage"
	"github.com/spf13/cobra"
)

var (
	target         string
	port           int
	captureHeaders bool
	captureBodies  bool
)

var proxyCmd = &cobra.Command{
	Use:   "proxy",
	Short: "Start a reverse proxy that captures API traffic",
	Long:  "Proxies all HTTP traffic to the target, capturing request/response metadata.",
	RunE: func(cmd *cobra.Command, args []string) error {
		if target == "" {
			return fmt.Errorf("--target is required")
		}

		db, err := storage.OpenDB(dbPath)
		if err != nil {
			return fmt.Errorf("failed to open database: %w", err)
		}
		defer db.Close()

		cfg := proxy.Config{
			Target:         target,
			Port:           port,
			CaptureHeaders: captureHeaders,
			CaptureBodies:  captureBodies,
		}

		log.Printf("Starting proxy on :%d -> %s", port, target)
		return proxy.Run(cmd.Context(), cfg, db)
	},
}

func init() {
	proxyCmd.Flags().StringVar(&target, "target", "", "target URL to proxy requests to (required)")
	proxyCmd.Flags().IntVar(&port, "port", 9000, "port to listen on")
	proxyCmd.Flags().BoolVar(&captureHeaders, "capture-headers", true, "capture request/response headers")
	proxyCmd.Flags().BoolVar(&captureBodies, "capture-bodies", true, "capture body schemas (inferred, not raw)")
	rootCmd.AddCommand(proxyCmd)
}
