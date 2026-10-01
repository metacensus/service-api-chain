// Package gateway implements store.Store as chaincode invocations over a Fabric gateway, encoded by internal/wire.
package gateway

import (
	"context"
	"fmt"

	"github.com/hyperledger/fabric-gateway/pkg/client"
	"google.golang.org/protobuf/proto"

	v1 "github.com/metacensus/api/go/metacensus/v1"
	"github.com/metacensus/api/go/store"

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

func newStore(c invoker) store.Store { return &chainStore{c: c} }

type chainStore struct{ c invoker }

func (s *chainStore) EnrollUser(ctx context.Context, record *v1.UserSigned, publicKey, passwordHash string) error {
	args, err := wire.EncodeEnrollUser(record, publicKey, passwordHash)
	if err != nil {
		return failure(wire.EnrollUser, err)
	}
	return s.submit(ctx, wire.EnrollUser, args)
}

func (s *chainStore) CreateTopic(ctx context.Context, callerID string, record *v1.TopicSigned) error {
	return s.write(ctx, wire.CreateTopic, callerID, record)
}

func (s *chainStore) CreateProp(ctx context.Context, callerID string, record *v1.PropSigned) error {
	return s.write(ctx, wire.CreateProp, callerID, record)
}

func (s *chainStore) SetVote(ctx context.Context, callerID string, record *v1.VoteSigned) error {
	return s.write(ctx, wire.SetVote, callerID, record)
}

func (s *chainStore) Credential(ctx context.Context, email string) (id, passwordHash string, err error) {
	out, err := s.evaluate(ctx, wire.Credential, email)
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
	return getRecord[*v1.UserSigned](ctx, s, wire.GetUser, id)
}

func (s *chainStore) GetTopic(ctx context.Context, id string) (*v1.TopicSigned, error) {
	return getRecord[*v1.TopicSigned](ctx, s, wire.GetTopic, id)
}

func (s *chainStore) GetProp(ctx context.Context, topicID, propID string) (*v1.PropSigned, error) {
	return getRecord[*v1.PropSigned](ctx, s, wire.GetProp, topicID, propID)
}

func (s *chainStore) ListUsers(ctx context.Context) ([]*v1.UserSigned, error) {
	return listRecords[*v1.UserSigned](ctx, s, wire.ListUsers)
}

func (s *chainStore) ListTopics(ctx context.Context) ([]*v1.TopicSigned, error) {
	return listRecords[*v1.TopicSigned](ctx, s, wire.ListTopics)
}

func (s *chainStore) ListProps(ctx context.Context, topicID string) ([]*v1.PropSigned, error) {
	return listRecords[*v1.PropSigned](ctx, s, wire.ListProps, topicID)
}

func (s *chainStore) ListVotes(ctx context.Context, topicID, propID string) ([]*v1.VoteSigned, error) {
	return listRecords[*v1.VoteSigned](ctx, s, wire.ListVotes, topicID, propID)
}

func (s *chainStore) write(ctx context.Context, op, callerID string, record proto.Message) error {
	args, err := wire.EncodeWrite(callerID, record)
	if err != nil {
		return failure(op, err)
	}
	return s.submit(ctx, op, args)
}

func (s *chainStore) submit(ctx context.Context, op string, args [][]byte) error {
	if _, err := s.c.Submit(ctx, op, args); err != nil {
		return failure(op, err)
	}
	return nil
}

func (s *chainStore) evaluate(ctx context.Context, op string, args ...string) ([]byte, error) {
	out, err := s.c.Evaluate(ctx, op, wire.EncodeStrings(args...))
	if err != nil {
		return nil, failure(op, err)
	}
	return out, nil
}

// newRecord makes an empty message of the record type R, a pointer type.
func newRecord[R proto.Message]() R {
	var r R
	return r.ProtoReflect().New().Interface().(R)
}

func getRecord[R proto.Message](ctx context.Context, s *chainStore, op string, args ...string) (R, error) {
	var zero R
	out, err := s.evaluate(ctx, op, args...)
	if err != nil {
		return zero, err
	}
	rec := newRecord[R]()
	if err := proto.Unmarshal(out, rec); err != nil {
		return zero, failure(op, err)
	}
	return rec, nil
}

func listRecords[R proto.Message](ctx context.Context, s *chainStore, op string, args ...string) ([]R, error) {
	out, err := s.evaluate(ctx, op, args...)
	if err != nil {
		return nil, err
	}
	items, err := wire.DecodeList(out)
	if err != nil {
		return nil, failure(op, err)
	}
	recs := make([]R, len(items))
	for i, item := range items {
		recs[i] = newRecord[R]()
		if err := proto.Unmarshal(item, recs[i]); err != nil {
			return nil, failure(op, err)
		}
	}
	return recs, nil
}

func failure(op string, err error) error {
	if kind := classify(err); kind != "" {
		return store.Errf(kind, op, err)
	}
	return fmt.Errorf("%s: %w", op, err)
}
