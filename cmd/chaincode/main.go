// Command chaincode serves internal/chaincode as Fabric chaincode-as-a-service.
package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/hyperledger/fabric-chaincode-go/v2/shim"

	"github.com/metacensus/api/go/signing"

	"github.com/metacensus/service-api-chain/internal/chaincode"
	"github.com/metacensus/service-api-chain/internal/config"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	if err := run(logger); err != nil {
		logger.Error("chaincode stopped", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.LoadChaincode(os.Getenv)
	if err != nil {
		return err
	}
	tlsProps, err := tlsProperties(cfg)
	if err != nil {
		return err
	}

	srv := &shim.ChaincodeServer{
		CCID:    cfg.ID,
		Address: cfg.Address,
		CC: chaincode.New(signing.ParticipantPolicy(cfg.AllowedOrigins...), chaincode.Config{
			GlobalChannel:   cfg.GlobalChannel,
			GlobalChaincode: cfg.GlobalChaincode,
		}),
		TLSProps: tlsProps,
	}
	logger.Info("serving", slog.String("ccid", cfg.ID), slog.String("addr", cfg.Address), slog.Bool("tls", !cfg.Plaintext))
	return srv.Start()
}

func tlsProperties(cfg config.Chaincode) (shim.TLSProperties, error) {
	if cfg.Plaintext {
		return shim.TLSProperties{Disabled: true}, nil
	}
	var p shim.TLSProperties
	for _, f := range []struct {
		path string
		into *[]byte
	}{{cfg.TLSCertFile, &p.Cert}, {cfg.TLSKeyFile, &p.Key}, {cfg.TLSClientCAFile, &p.ClientCACerts}} {
		if f.path == "" {
			continue
		}
		b, err := os.ReadFile(f.path)
		if err != nil {
			return shim.TLSProperties{}, fmt.Errorf("reading %s: %w", f.path, err)
		}
		*f.into = b
	}
	return p, nil
}
