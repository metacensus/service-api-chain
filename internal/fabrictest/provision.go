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
)

// Microfab's orderer runs a system channel ("testchainid", solo consensus),
// so a channel is created the classic way: a signed channel-creation update
// broadcast to the orderer, which fills the orderer and consortium groups from
// the system channel. consortium is the one its genesis block names, and Org1's
// group in it is keyed by MSP ID.
const consortium = "SampleConsortium"

// Provisioner is channels.PerTopic's create: Provision with name@pkgID.
func (f *Fab) Provisioner(name, pkgID string) func(ctx context.Context, channel string) error {
	return func(ctx context.Context, channel string) error { return f.Provision(ctx, channel, name, pkgID) }
}

// Provision brings channel into being and defines name@pkgID (installed by
// Define) on it: created on the orderer, joined by the peer, approved and
// committed. Each step that finds its work already done is success, so a
// re-run finishes a partial one. It returns once the definition is committed,
// at which point the peer answers an invocation; measured on Microfab, the
// whole takes about 380 ms, three of them orderer batch timeouts.
func (f *Fab) Provision(ctx context.Context, channel, name, pkgID string) error {
	if err := f.createChannel(ctx, channel); err != nil && !already(err, "existing channel") {
		return fmt.Errorf("create %s: %w", channel, err)
	}
	block, err := f.genesisBlock(ctx, channel)
	if err != nil {
		return fmt.Errorf("fetch %s's genesis block: %w", channel, err)
	}
	if err := f.join(ctx, block); err != nil && !already(err, "already exists") {
		return fmt.Errorf("join %s: %w", channel, err)
	}
	if err := f.define(ctx, channel, name, pkgID); err != nil {
		return fmt.Errorf("define %s on %s: %w", name, channel, err)
	}
	return nil
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

func (f *Fab) join(ctx context.Context, block *cb.Block) error {
	return channel.JoinChannel(ctx, f.conn, f.admin, block)
}

// already reports whether err is Fabric saying the step's work exists: the
// orderer refuses an update to an "existing channel", the peer a ledger that
// "already exists", the lifecycle a re-approval that would "redefine the
// current committed sequence" and a re-commit whose "new definition must be
// sequence 2".
func already(err error, mark string) bool { return strings.Contains(err.Error(), mark) }
