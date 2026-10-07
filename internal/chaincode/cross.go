package chaincode

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/hyperledger/fabric-chaincode-go/v2/shim"

	"github.com/metacensus/api/go/store"

	"github.com/metacensus/service-api-chain/internal/config"
	"github.com/metacensus/service-api-chain/internal/wire"
)

// Remote is ledger.Global for a topic channel: it runs fn on the global
// channel, failing with store.NotFound for an absent record.
type Remote func(fn string, args ...string) ([]byte, error)

func (query Remote) Key(keyID string) (owner, publicKey string, found bool, err error) {
	out, err := query(wire.GetKey, keyID)
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

func (query Remote) HasTopic(topicID string) (bool, error) {
	_, err := query(wire.GetTopic, topicID)
	if errors.Is(err, store.NotFound) {
		return false, nil
	}
	return err == nil, err
}

func invokeOnGlobal(stub shim.ChaincodeStubInterface, cfg config.Global) Remote {
	return func(fn string, args ...string) ([]byte, error) {
		invocation := append([][]byte{[]byte(fn)}, wire.EncodeStrings(args...)...)
		resp := stub.InvokeChaincode(cfg.Chaincode, invocation, cfg.Channel)
		if resp.GetStatus() == http.StatusOK {
			return resp.GetPayload(), nil
		}
		cause := fmt.Errorf("%s on %s: %s", fn, cfg.Channel, resp.GetMessage())
		if kind := wire.ParseKind(resp.GetMessage()); kind != "" {
			return nil, store.Errf(kind, fn, cause)
		}
		return nil, cause
	}
}
