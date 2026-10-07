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
	"github.com/metacensus/service-api-chain/internal/ledger/memkv"
	"github.com/metacensus/service-api-chain/internal/wire"
)

var policy = signing.ParticipantPolicy(storetest.Origin)

// network is Fabric in process: a memkv per channel, failures surfaced as a
// peer surfaces them.
type network struct {
	mu     sync.Mutex
	states map[string]*memkv.Store
}

func newNetwork() *network {
	n := &network{states: map[string]*memkv.Store{}}
	for _, ch := range channels.Global {
		n.states[ch] = memkv.New()
	}
	return n
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
		out, err = chaincode.Dispatch(tx, chaincode.Remote(n.query), policy, name, args)
		return err
	})
	if err != nil {
		return nil, peerError(err)
	}
	return out, nil
}

func (n *network) query(channel, fn string, args ...string) ([]byte, error) {
	tx := n.state(channel).Begin()
	return chaincode.Dispatch(tx, chaincode.Remote(n.query), policy, fn, wire.EncodeStrings(args...))
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
	n := newNetwork()
	storetest.Run(t, storetest.Harness{
		Open:       func(*testing.T) store.Store { return newStore(n.contract, n.create) },
		Signatures: storetest.Hard,
	})
}

// noSuchChannel is the gateway's refusal to evaluate on a channel the peer has
// not joined, in the peer's words (TestStore_NoSuchChannel pins them).
func noSuchChannel(channel string) error {
	return status.Errorf(codes.Unavailable, "failed to get config for channel [%s]: could not get last config for channel %s", channel, channel)
}
