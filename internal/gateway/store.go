// Package gateway implements store.Store as chaincode invocations over a Fabric gateway, encoded by internal/wire.
package gateway

import (
	"context"
	"fmt"

	"github.com/hyperledger/fabric-gateway/pkg/client"
	"google.golang.org/protobuf/proto"

	v1 "github.com/metacensus/api/go/metacensus/v1"
	"github.com/metacensus/api/go/store"

	"github.com/metacensus/service-api-chain/internal/channels"
	"github.com/metacensus/service-api-chain/internal/wire"
)

// invoker is the slice of *client.Contract the store calls; tests fake it.
type invoker interface {
	Submit(ctx context.Context, name string, args [][]byte) ([]byte, error)
	Evaluate(ctx context.Context, name string, args [][]byte) ([]byte, error)
}

type contractInvoker struct{ c *client.Contract }

func (i contractInvoker) Submit(ctx context.Context, name string, args [][]byte) ([]byte, error) {
	return i.c.SubmitWithContext(ctx, name, client.WithBytesArguments(args...))
}

func (i contractInvoker) Evaluate(ctx context.Context, name string, args [][]byte) ([]byte, error) {
	return i.c.EvaluateWithContext(ctx, name, client.WithBytesArguments(args...))
}

func newStore(contract func(channel string) invoker, global string, topics channels.Channels) store.Store {
	return &chainStore{contract: contract, global: global, topics: topics}
}

type chainStore struct {
	contract func(channel string) invoker
	global   string
	topics   channels.Channels
}

func (s *chainStore) EnrollUser(ctx context.Context, record *v1.UserSigned, publicKey, passwordHash string) error {
	args, err := wire.EncodeEnrollUser(record, publicKey, passwordHash)
	if err != nil {
		return failure(wire.EnrollUser, err)
	}
	return s.submit(ctx, s.onGlobal(), wire.EnrollUser, args)
}

// CreateTopic provisions the topic's channel before the record, so a listed
// topic takes a prop at once (storetest relies on it). A refused record
// strands its channel; infra's orchestration will own the sequence.
func (s *chainStore) CreateTopic(ctx context.Context, callerID string, record *v1.TopicSigned) error {
	if err := s.topics.Provision(ctx, record.GetId()); err != nil {
		return failure(wire.CreateTopic, err)
	}
	return s.write(ctx, s.onGlobal(), wire.CreateTopic, callerID, record)
}

func (s *chainStore) CreateProp(ctx context.Context, callerID string, record *v1.PropSigned) error {
	return s.write(ctx, s.topic(record.GetContent().GetTopicId()), wire.CreateProp, callerID, record)
}

func (s *chainStore) SetVote(ctx context.Context, callerID string, record *v1.VoteSigned) error {
	return s.write(ctx, s.topic(record.GetContent().GetTopicId()), wire.SetVote, callerID, record)
}

func (s *chainStore) Credential(ctx context.Context, email string) (id, passwordHash string, err error) {
	out, err := s.evaluate(ctx, s.onGlobal(), wire.Credential, email)
	if err != nil {
		return "", "", err
	}
	items, err := wire.DecodeList(out)
	if err == nil && len(items) != 2 {
		err = fmt.Errorf("credential has %d items, want 2", len(items))
	}
	if err != nil {
		return "", "", failure(wire.Credential, err)
	}
	return string(items[0]), string(items[1]), nil
}

func (s *chainStore) GetUser(ctx context.Context, id string) (*v1.UserSigned, error) {
	return getRecord[*v1.UserSigned](ctx, s, s.onGlobal(), wire.GetUser, id)
}

func (s *chainStore) GetTopic(ctx context.Context, id string) (*v1.TopicSigned, error) {
	return getRecord[*v1.TopicSigned](ctx, s, s.onGlobal(), wire.GetTopic, id)
}

func (s *chainStore) GetProp(ctx context.Context, topicID, propID string) (*v1.PropSigned, error) {
	return getRecord[*v1.PropSigned](ctx, s, s.topic(topicID), wire.GetProp, topicID, propID)
}

func (s *chainStore) ListUsers(ctx context.Context) ([]*v1.UserSigned, error) {
	return listRecords[*v1.UserSigned](ctx, s, s.onGlobal(), wire.ListUsers)
}

func (s *chainStore) ListTopics(ctx context.Context) ([]*v1.TopicSigned, error) {
	return listRecords[*v1.TopicSigned](ctx, s, s.onGlobal(), wire.ListTopics)
}

func (s *chainStore) ListProps(ctx context.Context, topicID string) ([]*v1.PropSigned, error) {
	return listRecords[*v1.PropSigned](ctx, s, s.topic(topicID), wire.ListProps, topicID)
}

func (s *chainStore) ListVotes(ctx context.Context, topicID, propID string) ([]*v1.VoteSigned, error) {
	return listRecords[*v1.VoteSigned](ctx, s, s.topic(topicID), wire.ListVotes, topicID, propID)
}

// target is an op's channel. A topic's channel that cannot be resolved or is
// not served answers as the topic not existing: InvalidContent for a write,
// NotFound for a read. Under channels.Shared a topic's channel is the global
// one, whose refusal stays what it is.
type target struct {
	channel    string
	unresolved error
	onTopic    bool
}

func (s *chainStore) onGlobal() target { return target{channel: s.global} }

func (s *chainStore) topic(topicID string) target {
	channel, err := s.topics.Resolve(topicID)
	return target{channel: channel, unresolved: err, onTopic: channel != s.global}
}

func (s *chainStore) write(ctx context.Context, to target, op, callerID string, record proto.Message) error {
	args, err := wire.EncodeWrite(callerID, record)
	if err != nil {
		return failure(op, err)
	}
	return s.submit(ctx, to, op, args)
}

func (s *chainStore) submit(ctx context.Context, to target, op string, args [][]byte) error {
	if to.unresolved != nil {
		return store.Errf(store.InvalidContent, op, to.unresolved)
	}
	if _, err := s.contract(to.channel).Submit(ctx, op, args); err != nil {
		return to.failure(store.InvalidContent, op, err)
	}
	return nil
}

func (s *chainStore) evaluate(ctx context.Context, to target, op string, args ...string) ([]byte, error) {
	if to.unresolved != nil {
		return nil, store.Errf(store.NotFound, op, to.unresolved)
	}
	out, err := s.contract(to.channel).Evaluate(ctx, op, wire.EncodeStrings(args...))
	if err != nil {
		return nil, to.failure(store.NotFound, op, err)
	}
	return out, nil
}

func (to target) failure(missing store.Kind, op string, err error) error {
	if to.onTopic && missingChannel(err) {
		return store.Errf(missing, op, err)
	}
	return failure(op, err)
}

func getRecord[R proto.Message](ctx context.Context, s *chainStore, to target, op string, args ...string) (R, error) {
	var zero R
	out, err := s.evaluate(ctx, to, op, args...)
	if err != nil {
		return zero, err
	}
	rec, err := wire.DecodeRecord[R](out)
	if err != nil {
		return zero, failure(op, err)
	}
	return rec, nil
}

func listRecords[R proto.Message](ctx context.Context, s *chainStore, to target, op string, args ...string) ([]R, error) {
	out, err := s.evaluate(ctx, to, op, args...)
	if err != nil {
		return nil, err
	}
	recs, err := wire.DecodeRecords[R](out)
	if err != nil {
		return nil, failure(op, err)
	}
	return recs, nil
}

func failure(op string, err error) error {
	if kind := classify(err); kind != "" {
		return store.Errf(kind, op, err)
	}
	return fmt.Errorf("%s: %w", op, err)
}
