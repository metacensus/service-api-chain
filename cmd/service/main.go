// Command service serves the MetaCensus API over a Fabric-backed store.Store.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/metacensus/api/go/service"

	"github.com/metacensus/service-api-chain/internal/channels"
	"github.com/metacensus/service-api-chain/internal/config"
	"github.com/metacensus/service-api-chain/internal/gateway"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
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

	st, closeGateway, err := gateway.Connect(cfg.Fabric, channels.Shared(cfg.Fabric.Channel))
	if err != nil {
		return err
	}
	defer func() {
		if err := closeGateway(); err != nil {
			logger.Warn("closing the gateway", slog.String("error", err.Error()))
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	return service.New(service.Config{Store: st}).Serve(ctx, cfg.Port, logger)
}
