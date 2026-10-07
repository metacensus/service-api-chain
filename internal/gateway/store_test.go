package gateway

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"

	v1 "github.com/metacensus/api/go/metacensus/v1"
	"github.com/metacensus/api/go/store"

	"github.com/metacensus/service-api-chain/internal/wire"
)

const (
	topicID      = "topc:0192a642-817d-7a3e-a282-d7a282ebd482"
	topicChannel = "metacensus.topic.0192a642-817d-7a3e-a282-d7a282ebd482"
	propID       = "prop:0192a642-817d-7a3e-a282-d7a282ebd483"
)

var (
	errBoom = errors.New("boom")

	user  = &v1.UserSigned{Id: "u1"}
	topic = &v1.TopicSigned{Id: topicID}
	prop  = &v1.PropSigned{Id: propID, Content: &v1.Prop{TopicId: topicID}}
	vote  = &v1.VoteSigned{Content: &v1.Vote{TopicId: topicID, PropId: propID, UserId: "u1"}}
)

func mustRecord(t *testing.T, m proto.Message) []byte {
	t.Helper()
	b, err := wire.Record(m)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func mustList[R proto.Message](t *testing.T, recs []R) []byte {
	t.Helper()
	b, err := wire.EncodeRecords(recs)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func assertCall(t *testing.T, f *fakeContract, submit bool, name string, wantArgs [][]byte) {
	t.Helper()
	if len(f.calls) != 1 {
		t.Fatalf("%d invocations, want 1", len(f.calls))
	}
	c := f.calls[0]
	if c.submit != submit || c.name != name {
		t.Errorf("invoked %q (submit=%v), want %q (submit=%v)", c.name, c.submit, name, submit)
	}
	if !slices.EqualFunc(c.args, wantArgs, bytes.Equal) {
		t.Errorf("args = %q, want %q", c.args, wantArgs)
	}
}

func assertKind(t *testing.T, err error, want store.Kind) {
	t.Helper()
	if got := store.KindOf(err); got != want {
		t.Errorf("kind = %q, want %q (err: %v)", got, want, err)
	}
}

func TestStore_Writes(t *testing.T) {
	tests := []struct {
		name     string
		method   string
		call     func(s store.Store) error
		wantArgs func(t *testing.T) [][]byte
	}{
		{
			name:   "success - EnrollUser",
			method: wire.EnrollUser,
			call:   func(s store.Store) error { return s.EnrollUser(t.Context(), user, "pub", "hash") },
			wantArgs: func(t *testing.T) [][]byte {
				return [][]byte{mustRecord(t, user), []byte("pub"), []byte("hash")}
			},
		},
		{
			name:   "success - CreateTopic",
			method: wire.CreateTopic,
			call:   func(s store.Store) error { return s.CreateTopic(t.Context(), "u1", topic) },
			wantArgs: func(t *testing.T) [][]byte {
				return [][]byte{[]byte("u1"), mustRecord(t, topic)}
			},
		},
		{
			name:   "success - CreateProp",
			method: wire.CreateProp,
			call:   func(s store.Store) error { return s.CreateProp(t.Context(), "u1", prop) },
			wantArgs: func(t *testing.T) [][]byte {
				return [][]byte{[]byte("u1"), mustRecord(t, prop)}
			},
		},
		{
			name:   "success - SetVote",
			method: wire.SetVote,
			call:   func(s store.Store) error { return s.SetVote(t.Context(), "u1", vote) },
			wantArgs: func(t *testing.T) [][]byte {
				return [][]byte{[]byte("u1"), mustRecord(t, vote)}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeContract{}
			if err := tt.call(storeOver(f)); err != nil {
				t.Fatalf("err = %v", err)
			}
			assertCall(t, f, true, tt.method, tt.wantArgs(t))
		})

		t.Run("error - "+tt.method+" classified from the chaincode", func(t *testing.T) {
			f := &fakeContract{err: statusWithDetails(t, codes.Aborted, wire.ErrorMessage(store.AlreadyExists))}
			err := tt.call(storeOver(f))
			assertKind(t, err, store.AlreadyExists)
		})

		t.Run("error - "+tt.method+" unclassified", func(t *testing.T) {
			f := &fakeContract{err: errBoom}
			err := tt.call(storeOver(f))
			assertKind(t, err, "")
			if !errors.Is(err, errBoom) {
				t.Errorf("cause lost: %v", err)
			}
		})
	}
}

func TestStore_Credential(t *testing.T) {
	tests := []struct {
		name     string
		out      []byte
		err      error
		wantID   string
		wantHash string
		wantKind store.Kind
		wantErr  bool
	}{
		{name: "success - id and hash", out: wire.EncodeList([][]byte{[]byte("u1"), []byte("h")}), wantID: "u1", wantHash: "h"},
		{name: "error - one item", out: wire.EncodeList([][]byte{[]byte("u1")}), wantErr: true},
		{name: "error - malformed list", out: []byte{0xff}, wantErr: true},
		{name: "error - unauthenticated from the chaincode", err: statusWithDetails(t, codes.Aborted, wire.ErrorMessage(store.Unauthenticated)), wantKind: store.Unauthenticated, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeContract{out: tt.out, err: tt.err}
			id, hash, err := storeOver(f).Credential(t.Context(), "a@b.c")
			assertCall(t, f, false, wire.Credential, [][]byte{[]byte("a@b.c")})
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			assertKind(t, err, tt.wantKind)
			if id != tt.wantID || hash != tt.wantHash {
				t.Errorf("got (%q, %q), want (%q, %q)", id, hash, tt.wantID, tt.wantHash)
			}
		})
	}
}

type getCase[R proto.Message] struct {
	name   string
	method string
	call   func(s store.Store) (R, error)
	args   []string
	want   R
}

func runGet[R proto.Message](t *testing.T, tt getCase[R]) {
	t.Run("success - "+tt.name, func(t *testing.T) {
		f := &fakeContract{out: mustRecord(t, tt.want)}
		got, err := tt.call(storeOver(f))
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		assertCall(t, f, false, tt.method, wire.EncodeStrings(tt.args...))
		if !proto.Equal(got, tt.want) {
			t.Errorf("got %v, want %v", got, tt.want)
		}
	})
	t.Run("error - "+tt.name+" not found", func(t *testing.T) {
		f := &fakeContract{err: statusWithDetails(t, codes.Aborted, wire.ErrorMessage(store.NotFound))}
		_, err := tt.call(storeOver(f))
		assertKind(t, err, store.NotFound)
	})
	t.Run("error - "+tt.name+" undecodable result", func(t *testing.T) {
		f := &fakeContract{out: []byte{0xff, 0xff}}
		_, err := tt.call(storeOver(f))
		if err == nil {
			t.Fatal("err = nil")
		}
		assertKind(t, err, "")
	})
}

func TestStore_Get(t *testing.T) {
	runGet(t, getCase[*v1.UserSigned]{name: "GetUser", method: wire.GetUser, args: []string{"u1"}, want: user,
		call: func(s store.Store) (*v1.UserSigned, error) { return s.GetUser(t.Context(), "u1") }})
	runGet(t, getCase[*v1.TopicSigned]{name: "GetTopic", method: wire.GetTopic, args: []string{topicID}, want: topic,
		call: func(s store.Store) (*v1.TopicSigned, error) { return s.GetTopic(t.Context(), topicID) }})
	runGet(t, getCase[*v1.PropSigned]{name: "GetProp", method: wire.GetProp, args: []string{topicID, propID}, want: prop,
		call: func(s store.Store) (*v1.PropSigned, error) { return s.GetProp(t.Context(), topicID, propID) }})
}

type listCase[R proto.Message] struct {
	name   string
	method string
	call   func(s store.Store) ([]R, error)
	args   []string
	want   []R
}

func runList[R proto.Message](t *testing.T, tt listCase[R]) {
	t.Run("success - "+tt.name, func(t *testing.T) {
		f := &fakeContract{out: mustList(t, tt.want)}
		got, err := tt.call(storeOver(f))
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		assertCall(t, f, false, tt.method, wire.EncodeStrings(tt.args...))
		if !slices.EqualFunc(got, tt.want, func(a, b R) bool { return proto.Equal(a, b) }) {
			t.Errorf("got %v, want %v", got, tt.want)
		}
	})
	t.Run("success - "+tt.name+" empty", func(t *testing.T) {
		got, err := tt.call(storeOver(&fakeContract{}))
		if err != nil || len(got) != 0 {
			t.Errorf("got (%v, %v), want empty", got, err)
		}
	})
	t.Run("error - "+tt.name+" unavailable", func(t *testing.T) {
		_, err := tt.call(storeOver(&fakeContract{err: context.Canceled}))
		assertKind(t, err, store.Unavailable)
	})
	t.Run("error - "+tt.name+" malformed list", func(t *testing.T) {
		_, err := tt.call(storeOver(&fakeContract{out: []byte{0xff}}))
		if err == nil {
			t.Fatal("err = nil")
		}
		assertKind(t, err, "")
	})
}

func TestStore_List(t *testing.T) {
	runList(t, listCase[*v1.UserSigned]{name: "ListUsers", method: wire.ListUsers, want: []*v1.UserSigned{user, {Id: "u2"}},
		call: func(s store.Store) ([]*v1.UserSigned, error) { return s.ListUsers(t.Context()) }})
	runList(t, listCase[*v1.TopicSigned]{name: "ListTopics", method: wire.ListTopics, want: []*v1.TopicSigned{topic},
		call: func(s store.Store) ([]*v1.TopicSigned, error) { return s.ListTopics(t.Context()) }})
	runList(t, listCase[*v1.PropSigned]{name: "ListProps", method: wire.ListProps, args: []string{topicID}, want: []*v1.PropSigned{prop},
		call: func(s store.Store) ([]*v1.PropSigned, error) { return s.ListProps(t.Context(), topicID) }})
	runList(t, listCase[*v1.VoteSigned]{name: "ListVotes", method: wire.ListVotes, args: []string{topicID, propID}, want: []*v1.VoteSigned{vote},
		call: func(s store.Store) ([]*v1.VoteSigned, error) { return s.ListVotes(t.Context(), topicID, propID) }})
}
