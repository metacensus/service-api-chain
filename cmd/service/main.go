// Command service serves the MetaCensus API over a Fabric-backed store.Store.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/metacensus/api/go/server"
	"github.com/metacensus/api/go/server/routes"
	"github.com/metacensus/api/go/service"
	"github.com/metacensus/service-api-chain/internal/config"
	"github.com/metacensus/service-api-chain/internal/gateway"
)

const drainTimeout = 10 * time.Second

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	if err := run(logger); err != nil {
		logger.Error("service stopped", slog.String("error", err.Error()))
		os.Exit(1)
	}
	logger.Info("stopped cleanly")
}

func run(logger *slog.Logger) error {
	cfg, err := config.LoadService(os.Getenv)
	if err != nil {
		return err
	}

	st, closeGateway, err := gateway.Connect(cfg.Fabric)
	if err != nil {
		return err
	}
	defer func() {
		if err := closeGateway(); err != nil {
			logger.Warn("closing the gateway", slog.String("error", err.Error()))
		}
	}()

	mux := http.NewServeMux()
	service.New(service.Config{Store: st}).Register(server.StdMux{ServeMux: mux}, &server.Runtime{Prefix: routes.Prefix})
	// /healthz is outside the contract (api AGENTS.md, "Why /healthz is not in the contract").
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})

	srv := &http.Server{
		Addr:              ":" + strconv.Itoa(cfg.Port),
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.ListenAndServe() }()
	logger.Info("listening", slog.String("addr", srv.Addr))

	select {
	case err := <-serveErr:
		return err
	case <-ctx.Done():
	}

	logger.Info("draining")
	drainCtx, cancel := context.WithTimeout(context.Background(), drainTimeout)
	defer cancel()
	if err := srv.Shutdown(drainCtx); err != nil {
		return err
	}
	if err := <-serveErr; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
