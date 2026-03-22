package proxy

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type Config struct {
	Target         string
	Port           int
	CaptureHeaders bool
	CaptureBodies  bool
}

// Run starts the reverse proxy server with traffic capture.
func Run(ctx context.Context, cfg Config, db *sql.DB) error {
	targetURL, err := url.Parse(cfg.Target)
	if err != nil {
		return fmt.Errorf("invalid target URL: %w", err)
	}

	ct := NewCaptureTransport(db, cfg.CaptureHeaders, cfg.CaptureBodies)

	rp := httputil.NewSingleHostReverseProxy(targetURL)
	rp.Transport = ct

	originalDirector := rp.Director
	rp.Director = func(req *http.Request) {
		originalDirector(req)
		req.Host = targetURL.Host
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	mux.Handle("/", rp)

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Port),
		Handler: mux,
		BaseContext: func(l net.Listener) context.Context {
			return ctx
		},
	}

	// Graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		ct.Close()
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	case sig := <-sigCh:
		log.Printf("Received %v, shutting down...", sig)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		server.Shutdown(shutdownCtx)
		ct.Close()
		return nil
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		server.Shutdown(shutdownCtx)
		ct.Close()
		return nil
	}
}
