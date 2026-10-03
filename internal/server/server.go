// Package server runs the AXmiPic HTTP server with graceful shutdown.
package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/AXmishell/axmipic/internal/config"
)

// Server owns the HTTP listener lifecycle.
type Server struct {
	logger          *slog.Logger
	httpServer      *http.Server
	shutdownTimeout time.Duration
}

// New builds the HTTP server from configuration and the root handler.
func New(cfg config.Config, logger *slog.Logger, handler http.Handler) *Server {
	addr := net.JoinHostPort(cfg.Server.Host, strconv.Itoa(cfg.Server.Port))
	return &Server{
		logger: logger,
		httpServer: &http.Server{
			Addr:         addr,
			Handler:      handler,
			ReadTimeout:  time.Duration(cfg.Server.ReadTimeoutSec) * time.Second,
			WriteTimeout: time.Duration(cfg.Server.WriteTimeoutSec) * time.Second,
		},
		shutdownTimeout: time.Duration(cfg.Server.ShutdownTimeoutSec) * time.Second,
	}
}

// Run starts the server and blocks until ctx is cancelled, then shuts down
// gracefully.
func (s *Server) Run(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		s.logger.Info("http server listening", slog.String("addr", s.httpServer.Addr))
		if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("server: listen: %w", err)
			return
		}
		errCh <- nil
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	s.logger.Info("shutting down http server")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), s.shutdownTimeout)
	defer cancel()
	if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("server: shutdown: %w", err)
	}
	return nil
}
