// Package ledger is store.Store over Fabric world state. One is built per
// chaincode invocation, whose reads do not see its own writes, so every check
// precedes every Put. Signed records are stored as signing.Canonical bytes
// (protojson's output is not stable across builds) so endorsing
// peers write the same bytes, and nothing here reads a clock, draws
// randomness, or ranges over a map.
package ledger

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"iter"

	"github.com/metacensus/api/go/contract"
	v1 "github.com/metacensus/api/go/metacensus/v1"
	"github.com/metacensus/api/go/signing"
	"github.com/metacensus/api/go/store"
	"google.golang.org/protobuf/proto"
)

// KV is the world state as one invocation sees it: the seam instead of the
// shim's stub, which is too wide to fake and has no shimtest in
// fabric-chaincode-go/v2.
type KV interface {
	// Get returns nil, nil when the key is absent.
	Get(key string) ([]byte, error)
	Put(key string, value []byte) error
	// Scan returns the values under a partial composite key, in key order.
	Scan(objectType string, attrs ...string) (iter.Seq2[[]byte, error], error)
	// Key fails on an attribute the world state cannot hold.
	Key(objectType string, attrs ...string) (string, error)
}

const (
	userType  = "user"
	emailType = "email"
	keyType   = "key"
	topicType = "topic"
	propType  = "prop"
	voteType  = "vote"
)

type emailRecord struct {
	ID           string `json:"id"`
	PasswordHash string `json:"passwordHash"`
}

type keyRecord struct {
	Owner     string `json:"owner"`
	PublicKey string `json:"publicKey"`
}

// Global is the users-and-topics channel as any channel reads it: Local on
// that channel, a cross-channel query elsewhere. The query is outside the read
// set and not re-validated at commit, which is safe only while keys and topics
// are never changed or removed.
type Global interface {
	Key(keyID string) (owner, publicKey string, found bool, err error)
	HasTopic(topicID string) (bool, error)
}

func Local(kv KV) Global { return local{kv} }

type local struct{ kv KV }

func (g local) Key(keyID string) (owner, publicKey string, found bool, err error) {
	k, err := g.kv.Key(keyType, keyID)
	if err != nil {
		return "", "", false, nil // a key_id the world state cannot hold names no enrolled key
	}
	b, err := g.kv.Get(k)
	if err != nil || b == nil {
		return "", "", false, err
	}
	var rec keyRecord
	if err := json.Unmarshal(b, &rec); err != nil {
		return "", "", false, err
	}
	return rec.Owner, rec.PublicKey, true, nil
}

func (g local) HasTopic(topicID string) (bool, error) {
	k, err := g.kv.Key(topicType, topicID)
	if err != nil {
		return false, nil // an id the world state cannot hold names no topic
	}
	b, err := g.kv.Get(k)
	return b != nil, err
}

type ledger struct {
	kv     KV
	global Global
	policy signing.Policy
}

func New(kv KV, global Global, policy signing.Policy) store.Store {
	return &ledger{kv: kv, global: global, policy: policy}
}

type op string

func (o op) kind(k store.Kind, cause error) error { return store.Errf(k, string(o), cause) }
func (o op) wrap(err error) error                 { return fmt.Errorf("%s: %w", o, err) }

func (l *ledger) key(o op, objectType string, attrs ...string) (string, error) {
	k, err := l.kv.Key(objectType, attrs...)
	if err != nil {
		return "", o.kind(store.InvalidContent, err)
	}
	return k, nil
}

func (l *ledger) get(o op, key string) ([]byte, error) {
	b, err := l.kv.Get(key)
	if err != nil {
		return nil, o.wrap(err)
	}
	return b, nil
}

func decode(b []byte, v any) error {
	if m, ok := v.(proto.Message); ok {
		return contract.Unmarshal(b, m)
	}
	return json.Unmarshal(b, v)
}

func (l *ledger) load(o op, key string, v any) (bool, error) {
	b, err := l.get(o, key)
	if err != nil || b == nil {
		return false, err
	}
	if err := decode(b, v); err != nil {
		return false, o.wrap(err)
	}
	return true, nil
}

func (l *ledger) exists(o op, key string) (bool, error) {
	b, err := l.get(o, key)
	return b != nil, err
}

func (l *ledger) free(o op, key string) error {
	taken, err := l.exists(o, key)
	if err != nil {
		return err
	}
	if taken {
		return o.kind(store.AlreadyExists, nil)
	}
	return nil
}

func (l *ledger) put(o op, key string, v any) error {
	var b []byte
	var err error
	if m, ok := v.(proto.Message); ok {
		b, err = signing.Canonical(m)
	} else {
		b, err = json.Marshal(v)
	}
	if err == nil {
		err = l.kv.Put(key, b)
	}
	if err != nil {
		return o.wrap(err)
	}
	return nil
}

func newOf[M proto.Message]() M {
	var none M
	return none.ProtoReflect().New().Interface().(M)
}

func list[M proto.Message](l *ledger, o op, objectType string, attrs ...string) ([]M, error) {
	if _, err := l.key(o, objectType, attrs...); err != nil {
		return nil, err
	}
	seq, err := l.kv.Scan(objectType, attrs...)
	if err != nil {
		return nil, o.wrap(err)
	}
	var out []M
	for b, err := range seq {
		if err != nil {
			return nil, o.wrap(err)
		}
		m := newOf[M]()
		if err := decode(b, m); err != nil {
			return nil, o.wrap(err)
		}
		out = append(out, m)
	}
	return out, nil
}

func read[M proto.Message](l *ledger, o op, objectType string, attrs ...string) (M, error) {
	var none M
	k, err := l.key(o, objectType, attrs...)
	if err != nil {
		return none, err
	}
	m := newOf[M]()
	found, err := l.load(o, k, m)
	if err != nil {
		return none, err
	}
	if !found {
		return none, o.kind(store.NotFound, nil)
	}
	return m, nil
}

// validID refuses an id that is not "<kind>:<UUIDv7>". The ledger is where the
// store contract is authoritative, so this is checked here, not in the gateway.
func validID(o op, kind store.IDKind, id string) error {
	if _, err := store.ParseID(kind, id); err != nil {
		return o.kind(store.InvalidContent, err)
	}
	return nil
}

func (l *ledger) authorize(o op, callerID string, content proto.Message, interp *v1.Interpretation, sig *v1.Signature) error {
	owner, publicKey, found, err := l.global.Key(sig.GetKeyId())
	if err != nil {
		return o.wrap(err)
	}
	if !found || owner != callerID {
		return o.kind(store.Unauthenticated, nil)
	}
	pub, err := signing.DecodePublicKey(publicKey)
	if err != nil {
		return o.wrap(err)
	}
	return l.stands(o, pub, content, interp, sig)
}

func (l *ledger) stands(o op, pub *ecdsa.PublicKey, content proto.Message, interp *v1.Interpretation, sig *v1.Signature) error {
	if err := signing.VerifyUser(pub, content, interp, sig, l.policy); err != nil {
		return o.kind(store.SignatureInvalid, err)
	}
	return nil
}

func (l *ledger) EnrollUser(_ context.Context, record *v1.UserSigned, publicKey, passwordHash string) error {
	const o = op("EnrollUser")
	if err := validID(o, store.UserID, record.GetId()); err != nil {
		return err
	}
	sig := record.GetUserSignature()
	pub, err := signing.EnrolledKey(publicKey, sig.GetKeyId())
	if err != nil {
		return o.kind(store.InvalidContent, err)
	}
	if err := l.stands(o, pub, record.GetContent(), record.GetInterpretation(), sig); err != nil {
		return err
	}
	emailKey, err := l.key(o, emailType, record.GetContent().GetEmail())
	if err != nil {
		return err
	}
	userKey, err := l.key(o, userType, record.GetId())
	if err != nil {
		return err
	}
	keyKey, err := l.key(o, keyType, sig.GetKeyId())
	if err != nil {
		return err
	}
	for _, k := range []string{emailKey, userKey, keyKey} {
		if err := l.free(o, k); err != nil {
			return err
		}
	}
	if err := l.put(o, userKey, record); err != nil {
		return err
	}
	if err := l.put(o, emailKey, emailRecord{ID: record.GetId(), PasswordHash: passwordHash}); err != nil {
		return err
	}
	return l.put(o, keyKey, keyRecord{Owner: record.GetId(), PublicKey: publicKey})
}

func (l *ledger) Credential(_ context.Context, email string) (string, string, error) {
	const o = op("Credential")
	k, err := l.key(o, emailType, email)
	if err != nil {
		return "", "", err
	}
	var rec emailRecord
	found, err := l.load(o, k, &rec)
	if err != nil {
		return "", "", err
	}
	if !found {
		return "", "", o.kind(store.Unauthenticated, nil)
	}
	return rec.ID, rec.PasswordHash, nil
}

func (l *ledger) GetUser(_ context.Context, id string) (*v1.UserSigned, error) {
	return read[*v1.UserSigned](l, "GetUser", userType, id)
}

func (l *ledger) ListUsers(_ context.Context) ([]*v1.UserSigned, error) {
	return list[*v1.UserSigned](l, "ListUsers", userType)
}

func (l *ledger) CreateTopic(_ context.Context, callerID string, record *v1.TopicSigned) error {
	const o = op("CreateTopic")
	if err := l.authorize(o, callerID, record.GetContent(), record.GetInterpretation(), record.GetUserSignature()); err != nil {
		return err
	}
	if err := validID(o, store.TopicID, record.GetId()); err != nil {
		return err
	}
	k, err := l.key(o, topicType, record.GetId())
	if err != nil {
		return err
	}
	if err := l.free(o, k); err != nil {
		return err
	}
	return l.put(o, k, record)
}

func (l *ledger) GetTopic(_ context.Context, id string) (*v1.TopicSigned, error) {
	return read[*v1.TopicSigned](l, "GetTopic", topicType, id)
}

func (l *ledger) ListTopics(_ context.Context) ([]*v1.TopicSigned, error) {
	return list[*v1.TopicSigned](l, "ListTopics", topicType)
}

func (l *ledger) CreateProp(_ context.Context, callerID string, record *v1.PropSigned) error {
	const o = op("CreateProp")
	if err := l.authorize(o, callerID, record.GetContent(), record.GetInterpretation(), record.GetUserSignature()); err != nil {
		return err
	}
	if err := validID(o, store.PropID, record.GetId()); err != nil {
		return err
	}
	topicID := record.GetContent().GetTopicId()
	hasTopic, err := l.global.HasTopic(topicID)
	if err != nil {
		return o.wrap(err)
	}
	if !hasTopic {
		return o.kind(store.InvalidContent, nil)
	}
	k, err := l.key(o, propType, topicID, record.GetId())
	if err != nil {
		return err
	}
	if err := l.free(o, k); err != nil {
		return err
	}
	return l.put(o, k, record)
}

func (l *ledger) GetProp(_ context.Context, topicID, propID string) (*v1.PropSigned, error) {
	return read[*v1.PropSigned](l, "GetProp", propType, topicID, propID)
}

func (l *ledger) ListProps(_ context.Context, topicID string) ([]*v1.PropSigned, error) {
	return list[*v1.PropSigned](l, "ListProps", propType, topicID)
}

func (l *ledger) SetVote(_ context.Context, callerID string, record *v1.VoteSigned) error {
	const o = op("SetVote")
	c := record.GetContent()
	if err := l.authorize(o, callerID, c, record.GetInterpretation(), record.GetUserSignature()); err != nil {
		return err
	}
	if c.GetUserId() == "" || c.GetUserId() != callerID {
		return o.kind(store.InvalidContent, nil)
	}
	propKey, err := l.key(o, propType, c.GetTopicId(), c.GetPropId())
	if err != nil {
		return err
	}
	hasProp, err := l.exists(o, propKey)
	if err != nil {
		return err
	}
	if !hasProp {
		return o.kind(store.InvalidContent, nil)
	}
	k, err := l.key(o, voteType, c.GetTopicId(), c.GetPropId(), c.GetUserId())
	if err != nil {
		return err
	}
	return l.put(o, k, record)
}

func (l *ledger) ListVotes(_ context.Context, topicID, propID string) ([]*v1.VoteSigned, error) {
	return list[*v1.VoteSigned](l, "ListVotes", voteType, topicID, propID)
}
