package ledger_test

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	v1 "github.com/metacensus/api/go/metacensus/v1"
	"github.com/metacensus/api/go/store"

	"github.com/metacensus/service-api-chain/internal/ledger"
	"github.com/metacensus/service-api-chain/internal/ledger/memkv"
)

var ctx = context.Background()

// unholdable is an id no world state can hold in a composite key.
const unholdable = "bad\x00id"

func TestLedger_EnrollUser(t *testing.T) {
	ada := newPerson(t, "ada")
	nulUserID := newPerson(t, "nul")
	nulUserID.user.Id = unholdable
	topicKindID := newPerson(t, "carl")
	topicKindID.user.Id = store.NewID(store.TopicID)
	notV7 := newPerson(t, "dee")
	notV7.user.Id = "user:00000000-0000-4000-8000-000000000000"
	nulEmail := newPerson(t, "bob")
	nulEmail.user.Content.Email = "a\x00b@example.test"
	nulEmail.user.Interpretation, nulEmail.user.UserSignature = nulEmail.sign(t, nulEmail.user.Content)

	tests := []struct {
		name string
		who  *person
		want store.Kind
		then func(t *testing.T, w *world)
	}{
		{name: "error - email the world state cannot hold", who: nulEmail, want: store.InvalidContent},
		{name: "error - id the world state cannot hold", who: nulUserID, want: store.InvalidContent},
		{name: "error - id of another kind", who: topicKindID, want: store.InvalidContent},
		{name: "error - id that is not a UUIDv7", who: notV7, want: store.InvalidContent},
		{
			name: "success - stores the user as canonical JSON, the email and the key as JSON",
			who:  ada,
			then: func(t *testing.T, w *world) {
				id := ada.user.GetId()
				assertRaw(t, w.raw("user", id), string(canonical(t, ada.user)))
				assertRaw(t, w.raw("email", "ada@example.test"), fmt.Sprintf(`{"id":%q,"passwordHash":%q}`, id, ada.hash))
				assertRaw(t, w.raw("key", ada.KeyID), fmt.Sprintf(`{"owner":%q,"publicKey":%q}`, id, ada.PublicKey))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t)
			err := w.invoke(func(s store.Store) error { return s.EnrollUser(ctx, tt.who.user, tt.who.PublicKey, tt.who.hash) })
			assertKind(t, err, tt.want)
			if tt.then != nil {
				tt.then(t, w)
			}
		})
	}
}

func TestLedger_Reads(t *testing.T) {
	tests := []struct {
		name string
		read func(s store.Store) error
	}{
		{name: "error - Credential email the world state cannot hold", read: func(s store.Store) error { _, _, err := s.Credential(ctx, unholdable); return err }},
		{name: "error - GetUser id the world state cannot hold", read: func(s store.Store) error { _, err := s.GetUser(ctx, unholdable); return err }},
		{name: "error - GetTopic id the world state cannot hold", read: func(s store.Store) error { _, err := s.GetTopic(ctx, unholdable); return err }},
		{name: "error - GetProp topic the world state cannot hold", read: func(s store.Store) error { _, err := s.GetProp(ctx, unholdable, "p"); return err }},
		{name: "error - GetProp prop the world state cannot hold", read: func(s store.Store) error { _, err := s.GetProp(ctx, "t", unholdable); return err }},
		{name: "error - ListProps topic the world state cannot hold", read: func(s store.Store) error { _, err := s.ListProps(ctx, unholdable); return err }},
		{name: "error - ListVotes prop the world state cannot hold", read: func(s store.Store) error { _, err := s.ListVotes(ctx, "t", unholdable); return err }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertKind(t, newWorld(t).invoke(tt.read), store.InvalidContent)
		})
	}
}

func TestLedger_Writes(t *testing.T) {
	ada := newPerson(t, "ada")
	caller := ada.user.GetId()
	topicID := store.NewID(store.TopicID)
	propID := store.NewID(store.PropID)
	forgedKeyID := ada.topic(t, topicID)
	forgedKeyID.UserSignature.KeyId = unholdable
	topic := ada.topic(t, topicID)
	// A caller who is not the author is Unauthenticated whatever else is wrong
	// with the record, so a malformed id must not mask it.
	badIDForeign := ada.topic(t, "not-an-id")

	tests := []struct {
		name string
		call func(w *world, s store.Store) error
		want store.Kind
		then func(t *testing.T, w *world)
	}{
		{
			name: "error - CreateTopic key_id the world state cannot hold is Unauthenticated",
			call: func(w *world, s store.Store) error { return s.CreateTopic(ctx, caller, forgedKeyID) },
			want: store.Unauthenticated,
		},
		{
			name: "error - CreateTopic by a caller who is not the author is Unauthenticated whatever the id",
			call: func(w *world, s store.Store) error {
				return s.CreateTopic(ctx, store.NewID(store.UserID), badIDForeign)
			},
			want: store.Unauthenticated,
		},
		{
			name: "error - CreateTopic id the world state cannot hold",
			call: func(w *world, s store.Store) error { return s.CreateTopic(ctx, caller, ada.topic(t, unholdable)) },
			want: store.InvalidContent,
		},
		{
			name: "error - CreateTopic id that is not a topic id",
			call: func(w *world, s store.Store) error { return s.CreateTopic(ctx, caller, ada.topic(t, "t")) },
			want: store.InvalidContent,
		},
		{
			name: "error - CreateTopic id of another kind",
			call: func(w *world, s store.Store) error {
				return s.CreateTopic(ctx, caller, ada.topic(t, store.NewID(store.PropID)))
			},
			want: store.InvalidContent,
		},
		{
			name: "error - CreateProp by a caller who is not the author is Unauthenticated whatever the id",
			call: func(w *world, s store.Store) error {
				w.topic(ada, topicID)
				return s.CreateProp(ctx, store.NewID(store.UserID), ada.prop(t, topicID, "not-an-id"))
			},
			want: store.Unauthenticated,
		},
		{
			name: "error - CreateProp id that is not a prop id",
			call: func(w *world, s store.Store) error {
				w.topic(ada, topicID)
				return s.CreateProp(ctx, caller, ada.prop(t, topicID, "p"))
			},
			want: store.InvalidContent,
		},
		{
			name: "error - CreateProp id of another kind",
			call: func(w *world, s store.Store) error {
				w.topic(ada, topicID)
				return s.CreateProp(ctx, caller, ada.prop(t, topicID, store.NewID(store.TopicID)))
			},
			want: store.InvalidContent,
		},
		{
			name: "error - CreateProp topic id the world state cannot hold",
			call: func(w *world, s store.Store) error { return s.CreateProp(ctx, caller, ada.prop(t, unholdable, propID)) },
			want: store.InvalidContent,
		},
		{
			name: "error - CreateProp id the world state cannot hold",
			call: func(w *world, s store.Store) error {
				w.topic(ada, topicID)
				return s.CreateProp(ctx, caller, ada.prop(t, topicID, unholdable))
			},
			want: store.InvalidContent,
		},
		{
			name: "error - SetVote topic the world state cannot hold",
			call: func(w *world, s store.Store) error {
				return s.SetVote(ctx, caller, ada.vote(t, unholdable, propID, v1.Vote_For))
			},
			want: store.InvalidContent,
		},
		{
			name: "success - CreateTopic stores canonical JSON",
			call: func(w *world, s store.Store) error { return s.CreateTopic(ctx, caller, topic) },
			then: func(t *testing.T, w *world) { assertRaw(t, w.raw("topic", topicID), string(canonical(t, topic))) },
		},
		{
			name: "success - CreateProp is stored under (topic, prop)",
			call: func(w *world, s store.Store) error {
				w.topic(ada, topicID)
				return s.CreateProp(ctx, caller, ada.prop(t, topicID, propID))
			},
			then: func(t *testing.T, w *world) {
				if w.raw("prop", topicID, propID) == nil {
					t.Error("prop not stored under (topic, prop)")
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t)
			w.enroll(ada)
			err := w.invoke(func(s store.Store) error { return tt.call(w, s) })
			assertKind(t, err, tt.want)
			if tt.then != nil {
				tt.then(t, w)
			}
		})
	}
}

// What is not the ledger's to classify carries no Kind, so the layer above
// reads it as an internal failure, not a client error.
func TestLedger_BackendFailures(t *testing.T) {
	ada := newPerson(t, "ada")
	failing := func(*testing.T) ledger.KV { return failingKV{memkv.New().Begin()} }
	corrupt := func(objectType, attr string) func(*testing.T) ledger.KV {
		return func(t *testing.T) ledger.KV {
			w := newWorld(t)
			w.put(objectType, []byte("not json"), attr)
			return w.state.Begin()
		}
	}
	tests := []struct {
		name string
		kv   func(t *testing.T) ledger.KV
		call func(s store.Store) error
	}{
		{name: "error - GetUser over a failing KV", kv: failing, call: func(s store.Store) error { _, err := s.GetUser(ctx, "ada"); return err }},
		{name: "error - ListUsers over a failing KV", kv: failing, call: func(s store.Store) error { _, err := s.ListUsers(ctx); return err }},
		{name: "error - EnrollUser over a failing KV", kv: failing, call: func(s store.Store) error { return s.EnrollUser(ctx, ada.user, ada.PublicKey, ada.hash) }},
		{name: "error - GetUser over a stored record that is not the contract's JSON", kv: corrupt("user", "ada"), call: func(s store.Store) error { _, err := s.GetUser(ctx, "ada"); return err }},
		{name: "error - Credential over a stored email record that is not JSON", kv: corrupt("email", "ada@example.test"), call: func(s store.Store) error { _, _, err := s.Credential(ctx, "ada@example.test"); return err }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kv := tt.kv(t)
			err := tt.call(ledger.New(kv, ledger.Local(kv), policy))
			if err == nil || store.KindOf(err) != "" {
				t.Fatalf("want an error without a Kind, got %v (Kind %q)", err, store.KindOf(err))
			}
		})
	}
}

func assertRaw(t *testing.T, got []byte, want string) {
	t.Helper()
	if !bytes.Equal(got, []byte(want)) {
		t.Errorf("stored %s, want %s", got, want)
	}
}
