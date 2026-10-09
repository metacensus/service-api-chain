package chaincode

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/proto"

	"github.com/metacensus/api/go/signing"
	"github.com/metacensus/api/go/store"

	v1 "github.com/metacensus/api/go/metacensus/v1"
	"github.com/metacensus/service-api-chain/internal/ledger"
	"github.com/metacensus/service-api-chain/internal/wire"
)

func Dispatch(kv ledger.KV, registry ledger.Registry, policy signing.Policy, fn string, args [][]byte) ([]byte, error) {
	ctx := context.Background() // the ledger never blocks; there is nothing to cancel
	s := ledger.New(kv, registry, policy)
	switch fn {
	case wire.EnrollUser:
		record, publicKey, hash, err := wire.DecodeEnrollUser(args)
		if err != nil {
			return nil, err
		}
		return nil, s.EnrollUser(ctx, record, publicKey, hash)
	case wire.CreateTopic:
		return write(ctx, args, &v1.TopicSigned{}, s.CreateTopic)
	case wire.CreateProp:
		return write(ctx, args, &v1.PropSigned{}, s.CreateProp)
	case wire.SetVote:
		return write(ctx, args, &v1.VoteSigned{}, s.SetVote)

	case wire.Credential:
		a, err := wire.DecodeStrings(args, 1)
		if err != nil {
			return nil, err
		}
		id, hash, err := s.Credential(ctx, a[0])
		if err != nil {
			return nil, err
		}
		return wire.EncodeList(wire.EncodeStrings(id, hash)), nil
	case wire.GetUser:
		return get(args, 1, func(a []string) (*v1.UserSigned, error) { return s.GetUser(ctx, a[0]) })
	case wire.GetTopic:
		return get(args, 1, func(a []string) (*v1.TopicSigned, error) { return s.GetTopic(ctx, a[0]) })
	case wire.GetProp:
		return get(args, 2, func(a []string) (*v1.PropSigned, error) { return s.GetProp(ctx, a[0], a[1]) })
	case wire.ListUsers:
		return list(args, 0, func([]string) ([]*v1.UserSigned, error) { return s.ListUsers(ctx) })
	case wire.ListTopics:
		return list(args, 0, func([]string) ([]*v1.TopicSigned, error) { return s.ListTopics(ctx) })
	case wire.ListProps:
		return list(args, 1, func(a []string) ([]*v1.PropSigned, error) { return s.ListProps(ctx, a[0]) })
	case wire.ListVotes:
		return list(args, 2, func(a []string) ([]*v1.VoteSigned, error) { return s.ListVotes(ctx, a[0], a[1]) })

	case wire.GetKey:
		a, err := wire.DecodeStrings(args, 1)
		if err != nil {
			return nil, err
		}
		owner, publicKey, found, err := ledger.Local(kv).Key(a[0])
		if err != nil {
			return nil, fmt.Errorf("%s: %w", fn, err)
		}
		if !found {
			return nil, store.Errf(store.NotFound, fn, nil)
		}
		return wire.EncodeList(wire.EncodeStrings(owner, publicKey)), nil
	}
	return nil, store.Errf(store.InvalidContent, fn, fmt.Errorf("unknown transaction"))
}

func write[R proto.Message](ctx context.Context, args [][]byte, record R, f func(context.Context, string, R) error) ([]byte, error) {
	callerID, err := wire.DecodeWrite(args, record)
	if err != nil {
		return nil, err
	}
	return nil, f(ctx, callerID, record)
}

func get[R proto.Message](args [][]byte, n int, f func(a []string) (R, error)) ([]byte, error) {
	a, err := wire.DecodeStrings(args, n)
	if err != nil {
		return nil, err
	}
	rec, err := f(a)
	if err != nil {
		return nil, err
	}
	return wire.Record(rec)
}

func list[R proto.Message](args [][]byte, n int, f func(a []string) ([]R, error)) ([]byte, error) {
	a, err := wire.DecodeStrings(args, n)
	if err != nil {
		return nil, err
	}
	recs, err := f(a)
	if err != nil {
		return nil, err
	}
	return wire.EncodeRecords(recs)
}
