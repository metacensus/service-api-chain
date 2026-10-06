package gateway

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	v1 "github.com/metacensus/api/go/metacensus/v1"
	"github.com/metacensus/api/go/store"

	"github.com/metacensus/service-api-chain/internal/channels"
	"github.com/metacensus/service-api-chain/internal/wire"
)

const (
	topicID      = "topc:0192a642-817d-7a3e-a282-d7a282ebd482"
	topicChannel = "metacensus.topic.0192a642-817d-7a3e-a282-d7a282ebd482"
	propID       = "prop:0192a642-817d-7a3e-a282-d7a282ebd483"
)

var (
	routedTopic = &v1.TopicSigned{Id: topicID}
	routedProp  = &v1.PropSigned{Id: propID, Content: &v1.Prop{TopicId: topicID}}
	routedVote  = &v1.VoteSigned{Content: &v1.Vote{TopicId: topicID, PropId: propID, UserId: "u1"}}
)

func perTopicStore(n *fakeNetwork, create func(context.Context, string) error) store.Store {
	return newStore(n.contract, globalChannel, channels.PerTopic(create))
}

func TestRouting(t *testing.T) {
	tests := []struct {
		name    string
		call    func(s store.Store) error
		channel string
		method  string
	}{
		{name: "EnrollUser on the global channel", channel: globalChannel, method: wire.EnrollUser,
			call: func(s store.Store) error { return s.EnrollUser(t.Context(), user, "pub", "hash") }},
		{name: "GetUser on the global channel", channel: globalChannel, method: wire.GetUser,
			call: func(s store.Store) error { _, err := s.GetUser(t.Context(), "u1"); return err }},
		{name: "GetTopic on the global channel", channel: globalChannel, method: wire.GetTopic,
			call: func(s store.Store) error { _, err := s.GetTopic(t.Context(), topicID); return err }},
		{name: "ListTopics on the global channel", channel: globalChannel, method: wire.ListTopics,
			call: func(s store.Store) error { _, err := s.ListTopics(t.Context()); return err }},
		{name: "CreateProp on the topic's channel", channel: topicChannel, method: wire.CreateProp,
			call: func(s store.Store) error { return s.CreateProp(t.Context(), "u1", routedProp) }},
		{name: "SetVote on the topic's channel", channel: topicChannel, method: wire.SetVote,
			call: func(s store.Store) error { return s.SetVote(t.Context(), "u1", routedVote) }},
		{name: "GetProp on the topic's channel", channel: topicChannel, method: wire.GetProp,
			call: func(s store.Store) error { _, err := s.GetProp(t.Context(), topicID, propID); return err }},
		{name: "ListProps on the topic's channel", channel: topicChannel, method: wire.ListProps,
			call: func(s store.Store) error { _, err := s.ListProps(t.Context(), topicID); return err }},
		{name: "ListVotes on the topic's channel", channel: topicChannel, method: wire.ListVotes,
			call: func(s store.Store) error { _, err := s.ListVotes(t.Context(), topicID, propID); return err }},
	}
	for _, tt := range tests {
		t.Run("success - "+tt.name, func(t *testing.T) {
			n := &fakeNetwork{}
			// Every read decodes an empty record or list, which is the empty answer.
			if err := tt.call(perTopicStore(n, nil)); err != nil {
				t.Fatalf("err = %v", err)
			}
			if len(n.asked) != 1 || n.asked[0] != tt.channel {
				t.Fatalf("invoked on %q, want [%q]", n.asked, tt.channel)
			}
			if calls := n.contracts[tt.channel].calls; len(calls) != 1 || calls[0].name != tt.method {
				t.Errorf("calls = %+v, want one %s", calls, tt.method)
			}
		})
	}
}

func TestRouting_CreateTopic(t *testing.T) {
	t.Run("success - the channel is provisioned, then the record written on the global channel", func(t *testing.T) {
		n := &fakeNetwork{}
		var order []string
		create := func(_ context.Context, ch string) error { order = append(order, "provision "+ch); return nil }
		if err := perTopicStore(n, create).CreateTopic(t.Context(), "u1", routedTopic); err != nil {
			t.Fatal(err)
		}
		for _, a := range n.asked {
			order = append(order, "write on "+a)
		}
		want := []string{"provision " + topicChannel, "write on " + globalChannel}
		if len(order) != 2 || order[0] != want[0] || order[1] != want[1] {
			t.Errorf("order = %q, want %q", order, want)
		}
	})

	t.Run("error - a failed provision writes nothing", func(t *testing.T) {
		n := &fakeNetwork{}
		create := func(context.Context, string) error { return context.DeadlineExceeded }
		err := perTopicStore(n, create).CreateTopic(t.Context(), "u1", routedTopic)
		assertKind(t, err, store.Unavailable)
		if len(n.asked) != 0 {
			t.Errorf("invoked on %q after a failed provision", n.asked)
		}
	})

	t.Run("error - an id that is not a topic id is InvalidContent before anything runs", func(t *testing.T) {
		n := &fakeNetwork{}
		create := func(context.Context, string) error { t.Fatal("provisioned"); return nil }
		err := perTopicStore(n, create).CreateTopic(t.Context(), "u1", &v1.TopicSigned{Id: "elections"})
		assertKind(t, err, store.InvalidContent)
	})
}

func TestRouting_NoSuchTopic(t *testing.T) {
	const bad = "user:0192a642-817d-7a3e-a282-d7a282ebd482"
	tests := []struct {
		name string
		call func(s store.Store) error
		want store.Kind
	}{
		{name: "CreateProp", want: store.InvalidContent,
			call: func(s store.Store) error {
				return s.CreateProp(t.Context(), "u1", &v1.PropSigned{Id: propID, Content: &v1.Prop{TopicId: bad}})
			}},
		{name: "SetVote", want: store.InvalidContent,
			call: func(s store.Store) error {
				return s.SetVote(t.Context(), "u1", &v1.VoteSigned{Content: &v1.Vote{TopicId: bad, PropId: propID, UserId: "u1"}})
			}},
		{name: "GetProp", want: store.NotFound,
			call: func(s store.Store) error { _, err := s.GetProp(t.Context(), bad, propID); return err }},
		{name: "ListProps", want: store.NotFound,
			call: func(s store.Store) error { _, err := s.ListProps(t.Context(), bad); return err }},
		{name: "ListVotes", want: store.NotFound,
			call: func(s store.Store) error { _, err := s.ListVotes(t.Context(), bad, propID); return err }},
	}
	for _, tt := range tests {
		t.Run("error - "+tt.name, func(t *testing.T) {
			n := &fakeNetwork{}
			err := tt.call(perTopicStore(n, nil))
			assertKind(t, err, tt.want)
			if !errors.Is(err, store.InvalidContent) {
				t.Errorf("the cause (not a topic id) is lost: %v", err)
			}
			if len(n.asked) != 0 {
				t.Errorf("invoked on %q", n.asked)
			}
		})
	}
}

func TestRouting_MissingChannel(t *testing.T) {
	refusals := map[string]error{
		"evaluate": status.Error(codes.Unavailable, "failed to get config for channel [x]: could not get last config for channel x"),
		"submit":   status.Error(codes.FailedPrecondition, "no combination of peers can be derived which satisfy the endorsement policy: No metadata was found for chaincode store in channel x"),
	}
	tests := []struct {
		name    string
		refusal string
		call    func(s store.Store) error
		want    store.Kind
	}{
		{name: "CreateProp is InvalidContent", refusal: "submit", want: store.InvalidContent,
			call: func(s store.Store) error { return s.CreateProp(t.Context(), "u1", routedProp) }},
		{name: "SetVote is InvalidContent", refusal: "submit", want: store.InvalidContent,
			call: func(s store.Store) error { return s.SetVote(t.Context(), "u1", routedVote) }},
		{name: "GetProp is NotFound", refusal: "evaluate", want: store.NotFound,
			call: func(s store.Store) error { _, err := s.GetProp(t.Context(), topicID, propID); return err }},
		{name: "ListProps is NotFound", refusal: "evaluate", want: store.NotFound,
			call: func(s store.Store) error { _, err := s.ListProps(t.Context(), topicID); return err }},
		{name: "ListVotes is NotFound", refusal: "evaluate", want: store.NotFound,
			call: func(s store.Store) error { _, err := s.ListVotes(t.Context(), topicID, propID); return err }},
		{name: "GetTopic on the global channel stays Unavailable", refusal: "evaluate", want: store.Unavailable,
			call: func(s store.Store) error { _, err := s.GetTopic(t.Context(), topicID); return err }},
		{name: "EnrollUser on the global channel stays unclassified", refusal: "submit", want: "",
			call: func(s store.Store) error { return s.EnrollUser(t.Context(), user, "pub", "hash") }},
	}
	for _, tt := range tests {
		t.Run("error - "+tt.name, func(t *testing.T) {
			f := &fakeContract{err: refusals[tt.refusal]}
			n := &fakeNetwork{contracts: map[string]*fakeContract{globalChannel: f, topicChannel: f}}
			err := tt.call(perTopicStore(n, nil))
			assertKind(t, err, tt.want)
			if !errors.Is(err, refusals[tt.refusal]) {
				t.Errorf("the peer's refusal is lost: %v", err)
			}
		})
	}

	t.Run("error - GetProp under Shared stays Unavailable", func(t *testing.T) {
		_, err := sharedStore(&fakeContract{err: refusals["evaluate"]}).GetProp(t.Context(), topicID, propID)
		assertKind(t, err, store.Unavailable)
	})
}
