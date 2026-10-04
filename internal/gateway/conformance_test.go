package gateway

import (
	"context"
	"sync"
	"testing"

	"github.com/hyperledger/fabric-protos-go-apiv2/gateway"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/metacensus/api/go/signing"
	"github.com/metacensus/api/go/store"
	"github.com/metacensus/api/go/store/storetest"

	"github.com/metacensus/service-api-chain/internal/chaincode"
	"github.com/metacensus/service-api-chain/internal/channels"
	"github.com/metacensus/service-api-chain/internal/ledger"
	"github.com/metacensus/service-api-chain/internal/ledger/memkv"
	"github.com/metacensus/service-api-chain/internal/wire"
)

var policy = signing.ParticipantPolicy(storetest.Origin)

// network is Fabric in process: a world state per channel, the chaincode
// dispatched over it with a topic channel's cross-channel reads answered by
// the global one, and a failure surfaced as a peer would surface it, the
// chaincode's message in an endorsement detail. A channel exists once created.
type network struct {
	mu     sync.Mutex
	states map[string]*memkv.Store
	global string
}

func newNetwork(global string) *network {
	return &network{states: map[string]*memkv.Store{global: memkv.New()}, global: global}
}

func (n *network) create(_ context.Context, channel string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if _, ok := n.states[channel]; !ok {
		n.states[channel] = memkv.New()
	}
	return nil
}

func (n *network) state(channel string) *memkv.Store {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.states[channel]
}

func (n *network) contract(channel string) invoker { return channelContract{n: n, channel: channel} }

type channelContract struct {
	n       *network
	channel string
}

func (c channelContract) Submit(_ context.Context, name string, args [][]byte) ([]byte, error) {
	return c.n.invoke(c.channel, name, args)
}

func (c channelContract) Evaluate(_ context.Context, name string, args [][]byte) ([]byte, error) {
	return c.n.invoke(c.channel, name, args)
}

func (n *network) invoke(channel, name string, args [][]byte) (out []byte, err error) {
	state := n.state(channel)
	if state == nil {
		return nil, noSuchChannel(channel)
	}
	err = state.Invoke(func(tx *memkv.Tx) (err error) {
		out, err = chaincode.Dispatch(tx, n.globalFor(channel, tx), policy, name, args)
		return err
	})
	if err != nil {
		return nil, peerError(err)
	}
	return out, nil
}

// globalFor is the users-and-topics state as channel reads it.
func (n *network) globalFor(channel string, tx *memkv.Tx) ledger.Global {
	if channel != n.global {
		return chaincode.Remote(n.query)
	}
	return ledger.Local(tx)
}

// query is a topic channel's read of the global channel: a transaction there
// that is never committed.
func (n *network) query(fn string, args ...string) ([]byte, error) {
	tx := n.state(n.global).Begin()
	return chaincode.Dispatch(tx, ledger.Local(tx), policy, fn, wire.EncodeStrings(args...))
}

func peerError(err error) error {
	s, derr := status.New(codes.Aborted, "failed to endorse transaction, see attached details for more info").
		WithDetails(&gateway.ErrorDetail{Message: "chaincode response 500, " + wire.ErrorMessage(err)})
	if derr != nil {
		return derr
	}
	return s.Err()
}

func TestConformance(t *testing.T) {
	t.Run("shared channel", func(t *testing.T) {
		n := newNetwork(globalChannel)
		storetest.Run(t, storetest.Harness{
			Open: func(*testing.T) store.Store {
				return newStore(n.contract, globalChannel, channels.Shared(globalChannel))
			},
			Signatures: storetest.Hard,
		})
	})
	t.Run("channel per topic", func(t *testing.T) {
		n := newNetwork(globalChannel)
		storetest.Run(t, storetest.Harness{
			Open:       func(*testing.T) store.Store { return newStore(n.contract, globalChannel, channels.PerTopic(n.create)) },
			Signatures: storetest.Hard,
		})
	})
}

// noSuchChannel is the gateway's refusal to evaluate on a channel the peer has
// not joined, in the peer's words (TestStore_NoSuchChannel pins them).
func noSuchChannel(channel string) error {
	return status.Errorf(codes.Unavailable, "failed to get config for channel [%s]: could not get last config for channel %s", channel, channel)
}
