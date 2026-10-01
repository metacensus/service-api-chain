package gateway

import (
	"context"
	"testing"

	"github.com/hyperledger/fabric-protos-go-apiv2/gateway"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/metacensus/api/go/signing"
	"github.com/metacensus/api/go/store"
	"github.com/metacensus/api/go/store/storetest"

	"github.com/metacensus/service-api-chain/internal/chaincode"
	"github.com/metacensus/service-api-chain/internal/ledger/memkv"
)

// inProcess is a channel in process; a failure surfaces as a peer would
// surface it, the chaincode's message in an endorsement detail.
type inProcess struct{ state *memkv.Store }

func (p inProcess) Submit(_ context.Context, name string, args [][]byte) ([]byte, error) {
	return p.invoke(name, args)
}

func (p inProcess) Evaluate(_ context.Context, name string, args [][]byte) ([]byte, error) {
	return p.invoke(name, args)
}

func (p inProcess) invoke(name string, args [][]byte) (out []byte, err error) {
	err = p.state.Invoke(func(tx *memkv.Tx) (err error) {
		out, err = chaincode.Dispatch(tx, signing.ParticipantPolicy(storetest.Origin), name, args)
		return err
	})
	if err != nil {
		return nil, peerError(err)
	}
	return out, nil
}

func peerError(err error) error {
	s, derr := status.New(codes.Aborted, "failed to endorse transaction, see attached details for more info").
		WithDetails(&gateway.ErrorDetail{Message: "chaincode response 500, " + chaincode.ErrorMessage(err)})
	if derr != nil {
		return derr
	}
	return s.Err()
}

func TestConformance(t *testing.T) {
	storetest.Run(t, storetest.Harness{
		Open:       func(*testing.T) store.Store { return newStore(inProcess{state: memkv.New()}) },
		Signatures: storetest.Hard,
	})
}
