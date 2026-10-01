package ledger_test

import (
	"context"
	"testing"

	v1 "github.com/metacensus/api/go/metacensus/v1"
	"github.com/metacensus/api/go/store"
	"github.com/metacensus/api/go/store/storetest"

	"github.com/metacensus/service-api-chain/internal/ledger"
	"github.com/metacensus/service-api-chain/internal/ledger/memkv"
)

// chain is a channel in process: one store call is one invocation.
type chain struct{ state *memkv.Store }

func invoke[T any](c *chain, f func(store.Store) (T, error)) (v T, err error) {
	err = c.state.Invoke(func(tx *memkv.Tx) (err error) {
		v, err = f(ledger.New(tx, policy))
		return err
	})
	return v, err
}

func invokeWrite(c *chain, f func(store.Store) error) error {
	_, err := invoke(c, func(s store.Store) (struct{}, error) { return struct{}{}, f(s) })
	return err
}

func (c *chain) EnrollUser(ctx context.Context, r *v1.UserSigned, publicKey, hash string) error {
	return invokeWrite(c, func(s store.Store) error { return s.EnrollUser(ctx, r, publicKey, hash) })
}

func (c *chain) Credential(ctx context.Context, email string) (id, hash string, err error) {
	type pair struct{ id, hash string }
	p, err := invoke(c, func(s store.Store) (pair, error) {
		id, hash, err := s.Credential(ctx, email)
		return pair{id, hash}, err
	})
	return p.id, p.hash, err
}

func (c *chain) GetUser(ctx context.Context, id string) (*v1.UserSigned, error) {
	return invoke(c, func(s store.Store) (*v1.UserSigned, error) { return s.GetUser(ctx, id) })
}

func (c *chain) ListUsers(ctx context.Context) ([]*v1.UserSigned, error) {
	return invoke(c, func(s store.Store) ([]*v1.UserSigned, error) { return s.ListUsers(ctx) })
}

func (c *chain) CreateTopic(ctx context.Context, callerID string, r *v1.TopicSigned) error {
	return invokeWrite(c, func(s store.Store) error { return s.CreateTopic(ctx, callerID, r) })
}

func (c *chain) GetTopic(ctx context.Context, id string) (*v1.TopicSigned, error) {
	return invoke(c, func(s store.Store) (*v1.TopicSigned, error) { return s.GetTopic(ctx, id) })
}

func (c *chain) ListTopics(ctx context.Context) ([]*v1.TopicSigned, error) {
	return invoke(c, func(s store.Store) ([]*v1.TopicSigned, error) { return s.ListTopics(ctx) })
}

func (c *chain) CreateProp(ctx context.Context, callerID string, r *v1.PropSigned) error {
	return invokeWrite(c, func(s store.Store) error { return s.CreateProp(ctx, callerID, r) })
}

func (c *chain) GetProp(ctx context.Context, topicID, propID string) (*v1.PropSigned, error) {
	return invoke(c, func(s store.Store) (*v1.PropSigned, error) { return s.GetProp(ctx, topicID, propID) })
}

func (c *chain) ListProps(ctx context.Context, topicID string) ([]*v1.PropSigned, error) {
	return invoke(c, func(s store.Store) ([]*v1.PropSigned, error) { return s.ListProps(ctx, topicID) })
}

func (c *chain) SetVote(ctx context.Context, callerID string, r *v1.VoteSigned) error {
	return invokeWrite(c, func(s store.Store) error { return s.SetVote(ctx, callerID, r) })
}

func (c *chain) ListVotes(ctx context.Context, topicID, propID string) ([]*v1.VoteSigned, error) {
	return invoke(c, func(s store.Store) ([]*v1.VoteSigned, error) { return s.ListVotes(ctx, topicID, propID) })
}

func TestConformance(t *testing.T) {
	storetest.Run(t, storetest.Harness{
		Open:       func(*testing.T) store.Store { return &chain{state: memkv.New()} },
		Signatures: storetest.Hard,
	})
}
