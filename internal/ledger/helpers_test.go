package ledger_test

import (
	"context"
	"errors"
	"iter"
	"testing"
	"time"

	v1 "github.com/metacensus/api/go/metacensus/v1"
	"github.com/metacensus/api/go/signing"
	"github.com/metacensus/api/go/store"
	"github.com/metacensus/api/go/store/storetest"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/metacensus/service-api-chain/internal/fabrictest"
	"github.com/metacensus/service-api-chain/internal/ledger"
	"github.com/metacensus/service-api-chain/internal/ledger/memkv"
)

var (
	policy = signing.ParticipantPolicy(storetest.Origin)
	at     = timestamppb.New(time.Unix(1_700_000_000, 123_456_789).UTC())
)

type person struct {
	fabrictest.Person
	hash string
	user *v1.UserSigned
}

func newPerson(t *testing.T, id string) *person {
	t.Helper()
	p := &person{Person: fabrictest.NewPerson(t), hash: "hash-of-" + id}
	content := &v1.User{Name: id, Email: id + "@example.test", Country: "GB"}
	interp, sig := p.sign(t, content)
	p.user = &v1.UserSigned{Id: id, Recorded: at, Content: content, Interpretation: interp, UserSignature: sig}
	return p
}

func (p *person) sign(t *testing.T, content proto.Message) (*v1.Interpretation, *v1.Signature) {
	return p.Sign(t, storetest.Origin, content)
}

func (p *person) topic(t *testing.T, id string) *v1.TopicSigned {
	t.Helper()
	content := &v1.Topic{Name: id, Description: "a topic"}
	interp, sig := p.sign(t, content)
	return &v1.TopicSigned{Id: id, Recorded: at, Content: content, Interpretation: interp, UserSignature: sig}
}

func (p *person) prop(t *testing.T, topicID, id string) *v1.PropSigned {
	t.Helper()
	content := &v1.Prop{TopicId: topicID, Type: v1.Prop_Statement, Description: "a prop"}
	interp, sig := p.sign(t, content)
	return &v1.PropSigned{Id: id, Recorded: at, Content: content, Interpretation: interp, UserSignature: sig}
}

func (p *person) vote(t *testing.T, topicID, propID string, pos v1.Vote_Position) *v1.VoteSigned {
	t.Helper()
	content := &v1.Vote{TopicId: topicID, PropId: propID, UserId: p.user.GetId(), Position: pos}
	interp, sig := p.sign(t, content)
	return &v1.VoteSigned{Recorded: at, Content: content, Interpretation: interp, UserSignature: sig}
}

type world struct {
	t     *testing.T
	state *memkv.Store
}

func newWorld(t *testing.T) *world { return &world{t: t, state: memkv.New()} }

func (w *world) invoke(f func(store.Store) error) error {
	return w.state.Invoke(func(tx *memkv.Tx) error { return f(ledger.New(tx, policy)) })
}

func (w *world) must(f func(store.Store) error) {
	w.t.Helper()
	if err := w.invoke(f); err != nil {
		w.t.Fatalf("setup: %v", err)
	}
}

func (w *world) enroll(p *person) {
	w.t.Helper()
	w.must(func(s store.Store) error { return s.EnrollUser(context.Background(), p.user, p.PublicKey, p.hash) })
}

func (w *world) topic(p *person, id string) *v1.TopicSigned {
	w.t.Helper()
	rec := p.topic(w.t, id)
	w.must(func(s store.Store) error { return s.CreateTopic(context.Background(), p.user.GetId(), rec) })
	return rec
}

func (w *world) raw(objectType string, attrs ...string) []byte {
	w.t.Helper()
	tx := w.state.Begin()
	k, err := tx.Key(objectType, attrs...)
	if err != nil {
		w.t.Fatal(err)
	}
	b, err := tx.Get(k)
	if err != nil {
		w.t.Fatal(err)
	}
	return b
}

func (w *world) put(objectType string, value []byte, attrs ...string) {
	w.t.Helper()
	tx := w.state.Begin()
	k, err := tx.Key(objectType, attrs...)
	if err != nil {
		w.t.Fatal(err)
	}
	if err := tx.Put(k, value); err != nil {
		w.t.Fatal(err)
	}
	tx.Commit()
}

func canonical(t *testing.T, m proto.Message) []byte {
	t.Helper()
	b, err := signing.Canonical(m)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func assertKind(t *testing.T, err error, want store.Kind) {
	t.Helper()
	if got := store.KindOf(err); got != want || (want == "" && err != nil) {
		t.Fatalf("want Kind %q, got %v (Kind %q)", want, err, got)
	}
}

type failingKV struct{ ledger.KV }

var errBackend = errors.New("state database unavailable")

func (failingKV) Get(string) ([]byte, error) { return nil, errBackend }
func (failingKV) Put(string, []byte) error   { return errBackend }
func (failingKV) Scan(string, ...string) (iter.Seq2[[]byte, error], error) {
	return nil, errBackend
}
