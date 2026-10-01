package chaincode_test

import (
	"testing"

	"github.com/metacensus/api/go/signing"
	"github.com/metacensus/api/go/store"
	"github.com/metacensus/api/go/store/storetest"

	"github.com/metacensus/service-api-chain/internal/chaincode"
	"github.com/metacensus/service-api-chain/internal/ledger/memkv"
	"github.com/metacensus/service-api-chain/internal/wire"
)

var policy = signing.ParticipantPolicy(storetest.Origin)

func dispatch(fn string, args [][]byte) (out []byte, err error) {
	err = memkv.New().Invoke(func(tx *memkv.Tx) (err error) {
		out, err = chaincode.Dispatch(tx, policy, fn, args)
		return err
	})
	return out, err
}

func TestDispatchRejects(t *testing.T) {
	tests := []struct {
		name string
		fn   string
		args [][]byte
	}{
		{name: "unknown transaction", fn: "DeleteEverything"},
		{name: "empty transaction name", fn: ""},
		{name: "GetUser without its id", fn: wire.GetUser},
		{name: "GetUser with an extra argument", fn: wire.GetUser, args: wire.EncodeStrings("a", "b")},
		{name: "GetProp with one argument", fn: wire.GetProp, args: wire.EncodeStrings("t")},
		{name: "ListUsers with an argument", fn: wire.ListUsers, args: wire.EncodeStrings("x")},
		{name: "Credential without an email", fn: wire.Credential},
		{name: "EnrollUser with two arguments", fn: wire.EnrollUser, args: wire.EncodeStrings("a", "b")},
		{name: "EnrollUser with an undecodable record", fn: wire.EnrollUser, args: [][]byte{{0xff, 0xff}, []byte("k"), []byte("h")}},
		{name: "CreateTopic with an undecodable record", fn: wire.CreateTopic, args: [][]byte{[]byte("u"), {0xff, 0xff}}},
		{name: "SetVote with one argument", fn: wire.SetVote, args: wire.EncodeStrings("u")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := dispatch(tt.fn, tt.args)
			if got := store.KindOf(err); got != store.InvalidContent {
				t.Fatalf("Kind = %q (err %v), want %q", got, err, store.InvalidContent)
			}
			if out != nil {
				t.Errorf("a refused transaction answered %q", out)
			}
		})
	}
}
