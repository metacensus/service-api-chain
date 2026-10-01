package chaincode

import (
	"iter"

	"github.com/hyperledger/fabric-chaincode-go/v2/shim"
)

type stateKV struct{ stub shim.ChaincodeStubInterface }

func (s stateKV) Get(key string) ([]byte, error) {
	b, err := s.stub.GetState(key)
	if len(b) == 0 {
		return nil, err
	}
	return b, err
}

func (s stateKV) Put(key string, value []byte) error { return s.stub.PutState(key, value) }

func (s stateKV) Key(objectType string, attrs ...string) (string, error) {
	return s.stub.CreateCompositeKey(objectType, attrs)
}

func (s stateKV) Scan(objectType string, attrs ...string) (iter.Seq2[[]byte, error], error) {
	it, err := s.stub.GetStateByPartialCompositeKey(objectType, attrs)
	if err != nil {
		return nil, err
	}
	return func(yield func([]byte, error) bool) {
		defer it.Close()
		for it.HasNext() {
			kv, err := it.Next()
			var v []byte
			if err == nil {
				v = kv.Value
			}
			if !yield(v, err) || err != nil {
				return
			}
		}
	}, nil
}
