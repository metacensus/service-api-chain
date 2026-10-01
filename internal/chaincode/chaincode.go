// Package chaincode runs internal/wire transactions against internal/ledger on a
// Fabric peer, and is held to ledger's determinism.
package chaincode

import (
	"github.com/hyperledger/fabric-chaincode-go/v2/shim"
	"github.com/hyperledger/fabric-protos-go-apiv2/peer"

	"github.com/metacensus/api/go/signing"

	"github.com/metacensus/service-api-chain/internal/wire"
)

func New(policy signing.Policy) shim.Chaincode { return &cc{policy: policy} }

type cc struct{ policy signing.Policy }

func (*cc) Init(shim.ChaincodeStubInterface) *peer.Response { return shim.Success(nil) }

func (c *cc) Invoke(stub shim.ChaincodeStubInterface) *peer.Response {
	fn, args := "", stub.GetArgs()
	if len(args) > 0 {
		fn, args = string(args[0]), args[1:]
	}
	out, err := Dispatch(stateKV{stub}, c.policy, fn, args)
	if err != nil {
		return shim.Error(wire.ErrorMessage(err))
	}
	return shim.Success(out)
}
