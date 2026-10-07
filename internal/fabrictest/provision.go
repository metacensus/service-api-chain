//go:build integration || artifact

package fabrictest

import (
	"context"
	"crypto/tls"
	"fmt"
	"strings"

	"github.com/hyperledger/fabric-admin-sdk/pkg/channel"
	"github.com/hyperledger/fabric-config/configtx"
	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	ab "github.com/hyperledger/fabric-protos-go-apiv2/orderer"

	"github.com/metacensus/service-api-chain/internal/channels"
)

// Microfab's orderer runs a system channel, so a channel is created the
// classic way, not by the participation API infra targets: a signed creation
// update broadcast to the orderer. consortium is the one its genesis block
// names; Org1's group in it is keyed by MSP ID.
const consortium = "SampleConsortium"

// Provisioner creates each topic's channel, joins the peer, and defines
// name@pkgID (installed by Define) there.
func (f *Fab) Provisioner(name, pkgID string) channels.Provision {
	return func(ctx context.Context, ch string) error {
		if err := f.createChannel(ctx, ch); err != nil && !already(err, "existing channel") {
			return fmt.Errorf("create %s: %w", ch, err)
		}
		block, err := f.genesisBlock(ctx, ch)
		if err != nil {
			return fmt.Errorf("fetch %s's genesis block: %w", ch, err)
		}
		if err := channel.JoinChannel(ctx, f.peer, f.admin, block); err != nil && !already(err, "already exists") {
			return fmt.Errorf("join %s: %w", ch, err)
		}
		return f.define(ctx, ch, name, pkgID)
	}
}

func (f *Fab) createChannel(ctx context.Context, name string) error {
	update, err := configtx.NewMarshaledCreateChannelTx(configtx.Channel{
		Consortium: consortium,
		Application: configtx.Application{
			Organizations: []configtx.Organization{{Name: MSPID}},
			Capabilities:  []string{"V2_0"},
			Policies: map[string]configtx.Policy{
				configtx.ReadersPolicyKey:              {Type: configtx.ImplicitMetaPolicyType, Rule: "ANY Readers"},
				configtx.WritersPolicyKey:              {Type: configtx.ImplicitMetaPolicyType, Rule: "ANY Writers"},
				configtx.AdminsPolicyKey:               {Type: configtx.ImplicitMetaPolicyType, Rule: "MAJORITY Admins"},
				configtx.EndorsementPolicyKey:          {Type: configtx.ImplicitMetaPolicyType, Rule: "MAJORITY Endorsement"},
				configtx.LifecycleEndorsementPolicyKey: {Type: configtx.ImplicitMetaPolicyType, Rule: "MAJORITY Endorsement"},
			},
		},
	}, name)
	if err != nil {
		return err
	}
	sig, err := f.signer.CreateConfigSignature(update)
	if err != nil {
		return err
	}
	env, err := configtx.NewEnvelope(update, sig)
	if err != nil {
		return err
	}
	if err := f.signer.SignEnvelope(env); err != nil {
		return err
	}
	bc, err := ab.NewAtomicBroadcastClient(f.orderer).Broadcast(ctx)
	if err != nil {
		return err
	}
	if err := bc.Send(env); err != nil {
		return err
	}
	if err := bc.CloseSend(); err != nil {
		return err
	}
	resp, err := bc.Recv()
	if err != nil {
		return err
	}
	if resp.GetStatus() != cb.Status_SUCCESS {
		return fmt.Errorf("orderer: %s: %s", resp.GetStatus(), resp.GetInfo())
	}
	return nil
}

// genesisBlock is the channel's block 0 from the orderer. The admin SDK
// derives a TLS binding from the certificate's first entry unconditionally,
// and the orderer ignores it without TLS, so one byte stands in.
func (f *Fab) genesisBlock(ctx context.Context, name string) (*cb.Block, error) {
	return channel.GetConfigBlockFromOrderer(ctx, f.orderer, f.admin, name, tls.Certificate{Certificate: [][]byte{{0}}})
}

// already reports whether err is Fabric refusing work that is already done.
func already(err error, mark string) bool { return strings.Contains(err.Error(), mark) }
