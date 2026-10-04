package chaincode

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/hyperledger/fabric-chaincode-go/v2/shim"

	"github.com/metacensus/api/go/store"

	"github.com/metacensus/service-api-chain/internal/ledger"
	"github.com/metacensus/service-api-chain/internal/wire"
)

// Remote is ledger.Global for a topic channel, answered by this chaincode's
// own transactions on the global channel: query runs fn there and returns what
// it returned, an error carrying the Kind (NotFound for an absent record).
func Remote(query func(fn string, args ...string) ([]byte, error)) ledger.Global {
	return remote{query: query}
}

type remote struct {
	query func(fn string, args ...string) ([]byte, error)
}

func (r remote) Key(keyID string) (owner, publicKey string, found bool, err error) {
	out, err := r.query(wire.GetKey, keyID)
	if errors.Is(err, store.NotFound) {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, err
	}
	items, err := wire.DecodeList(out)
	if err == nil && len(items) != 2 {
		err = fmt.Errorf("key has %d items, want 2", len(items))
	}
	if err != nil {
		return "", "", false, fmt.Errorf("%s: %w", wire.GetKey, err)
	}
	return string(items[0]), string(items[1]), true, nil
}

func (r remote) HasTopic(topicID string) (bool, error) {
	_, err := r.query(wire.GetTopic, topicID)
	if errors.Is(err, store.NotFound) {
		return false, nil
	}
	return err == nil, err
}

// invokeOnGlobal is Remote's query over the stub: a cross-channel invocation,
// which Fabric runs read-only and keeps out of this transaction's read set.
func invokeOnGlobal(stub shim.ChaincodeStubInterface, cfg Config) func(fn string, args ...string) ([]byte, error) {
	return func(fn string, args ...string) ([]byte, error) {
		invocation := append([][]byte{[]byte(fn)}, wire.EncodeStrings(args...)...)
		resp := stub.InvokeChaincode(cfg.GlobalChaincode, invocation, cfg.GlobalChannel)
		if resp.GetStatus() == http.StatusOK {
			return resp.GetPayload(), nil
		}
		cause := fmt.Errorf("%s on %s: %s", fn, cfg.GlobalChannel, resp.GetMessage())
		if kind := wire.ParseKind(resp.GetMessage()); kind != "" {
			return nil, store.Errf(kind, fn, cause)
		}
		return nil, cause
	}
}
