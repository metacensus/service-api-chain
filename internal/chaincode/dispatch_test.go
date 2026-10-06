package chaincode_test

import (
	"testing"

	"github.com/metacensus/api/go/signing"
	"github.com/metacensus/api/go/store"
	"github.com/metacensus/api/go/store/storetest"

	"github.com/metacensus/service-api-chain/internal/chaincode"
	"github.com/metacensus/service-api-chain/internal/fabrictest"
	"github.com/metacensus/service-api-chain/internal/ledger"
	"github.com/metacensus/service-api-chain/internal/ledger/memkv"
	"github.com/metacensus/service-api-chain/internal/wire"
)

var policy = signing.ParticipantPolicy(storetest.Origin)

func dispatch(fn string, args [][]byte) (out []byte, err error) {
	err = memkv.New().Invoke(func(tx *memkv.Tx) (err error) {
		out, err = chaincode.Dispatch(tx, ledger.Local(tx), policy, fn, args)
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

func TestDispatchGetKey(t *testing.T) {
	state := memkv.New()
	run := func(fn string, args [][]byte) (out []byte, err error) {
		err = state.Invoke(func(tx *memkv.Tx) (err error) {
			out, err = chaincode.Dispatch(tx, ledger.Local(tx), policy, fn, args)
			return err
		})
		return out, err
	}
	ada := fabrictest.NewPerson(t)
	user := ada.User(t, storetest.Origin, store.NewID(store.UserID), "ada@example.test")
	enroll, err := wire.EncodeEnrollUser(user, ada.PublicKey, "hash")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := run(wire.EnrollUser, enroll); err != nil {
		t.Fatalf("enroll: %v", err)
	}

	t.Run("success - an enrolled key", func(t *testing.T) {
		out, err := run(wire.GetKey, wire.EncodeStrings(ada.KeyID))
		if err != nil {
			t.Fatal(err)
		}
		items, err := wire.DecodeList(out)
		if err != nil || len(items) != 2 || string(items[0]) != user.GetId() || string(items[1]) != ada.PublicKey {
			t.Fatalf("got (%q, %v), want [%s %s]", items, err, user.GetId(), ada.PublicKey)
		}
	})
	t.Run("error - an unknown key is NotFound", func(t *testing.T) {
		_, err := run(wire.GetKey, wire.EncodeStrings("nobody"))
		if store.KindOf(err) != store.NotFound {
			t.Fatalf("Kind = %q (err %v), want NotFound", store.KindOf(err), err)
		}
	})
	t.Run("error - without its key id", func(t *testing.T) {
		_, err := run(wire.GetKey, nil)
		if store.KindOf(err) != store.InvalidContent {
			t.Fatalf("Kind = %q (err %v), want InvalidContent", store.KindOf(err), err)
		}
	})
}
