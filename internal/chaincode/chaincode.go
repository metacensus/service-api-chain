// Package chaincode runs internal/wire transactions against internal/ledger on a
// Fabric peer, and is held to ledger's determinism.
package chaincode

import (
	"github.com/hyperledger/fabric-chaincode-go/v2/shim"
	"github.com/hyperledger/fabric-protos-go-apiv2/peer"

	"github.com/metacensus/api/go/signing"

	"github.com/metacensus/service-api-chain/internal/ledger"
	"github.com/metacensus/service-api-chain/internal/wire"
)

// Config names the channel holding users and topics and this chaincode's name
// there; an empty GlobalChannel makes every channel its own global.
type Config struct {
	GlobalChannel, GlobalChaincode string
}

func New(policy signing.Policy, cfg Config) shim.Chaincode { return &cc{policy: policy, cfg: cfg} }

type cc struct {
	policy signing.Policy
	cfg    Config
}

func (*cc) Init(shim.ChaincodeStubInterface) *peer.Response { return shim.Success(nil) }

func (c *cc) Invoke(stub shim.ChaincodeStubInterface) *peer.Response {
	fn, args := "", stub.GetArgs()
	if len(args) > 0 {
		fn, args = string(args[0]), args[1:]
	}
	kv := stateKV{stub}
	out, err := Dispatch(kv, c.global(stub, kv), c.policy, fn, args)
	if err != nil {
		return shim.Error(wire.ErrorMessage(err))
	}
	return shim.Success(out)
}

func (c *cc) global(stub shim.ChaincodeStubInterface, kv ledger.KV) ledger.Global {
	if c.cfg.GlobalChannel != "" && stub.GetChannelID() != c.cfg.GlobalChannel {
		return invokeOnGlobal(stub, c.cfg)
	}
	return ledger.Local(kv)
}
