package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"stage-rigging-clearance/internal/application"
	"stage-rigging-clearance/internal/audit"
	"stage-rigging-clearance/internal/store"
	"stage-rigging-clearance/internal/transport"
	"syscall"
	"time"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		slog.Error("服务退出", "error", err)
		os.Exit(1)
	}
}
func run(args []string) error {
	cfg, err := parseConfig(args)
	if err != nil {
		return err
	}
	dbPath := cfg.DBPath
	if cfg.Selfcheck {
		dbPath = ":memory:"
	}
	ctx := context.Background()
	repo, err := store.Open(ctx, dbPath)
	if err != nil {
		return err
	}
	defer repo.Close()
	app := application.NewService(repo, realClock{}, &ids{}, audit.NewDigester())
	handler := transport.NewServer(app).Handler()
	srv := &http.Server{Addr: cfg.Addr, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second}
	ln, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		return fmt.Errorf("监听 %s: %w", cfg.Addr, err)
	}
	serveErr := make(chan error, 1)
	go func() {
		err := srv.Serve(ln)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
		close(serveErr)
	}()
	slog.Info("舞台吊挂放行服务已启动", "addr", ln.Addr().String())
	if cfg.Selfcheck {
		return performSelfcheck(srv, "http://"+ln.Addr().String())
	}
	sigCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	select {
	case err := <-serveErr:
		if err != nil {
			return err
		}
	case <-sigCtx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
