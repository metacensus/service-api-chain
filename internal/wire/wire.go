// Package wire is the protocol between this service's two deployables: the
// gateway client in the API process, which encodes a store.Store call into a
// chaincode invocation, and the chaincode on the peers, which decodes it. A
// change here is a change to both images.
//
// A transaction is named for the store.Store method it carries and takes that
// method's arguments in order, without ctx: a record as Record encodes it,
// anything else as a UTF-8 string. A read answers with one Record, a list with
// EncodeRecords, and Credential with an EncodeList of [id, passwordHash].
package wire

import (
	"fmt"
	"strings"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"

	v1 "github.com/metacensus/api/go/metacensus/v1"
	"github.com/metacensus/api/go/store"
)

const (
	EnrollUser  = "EnrollUser"
	Credential  = "Credential"
	GetUser     = "GetUser"
	ListUsers   = "ListUsers"
	CreateTopic = "CreateTopic"
	GetTopic    = "GetTopic"
	ListTopics  = "ListTopics"
	CreateProp  = "CreateProp"
	GetProp     = "GetProp"
	ListProps   = "ListProps"
	SetVote     = "SetVote"
	ListVotes   = "ListVotes"

	// GetKey has no store method: it is ledger.Registry.Key, asked of metacensus.users.
	GetKey = "GetKey"
)

// Record encodes m deterministically, so endorsing peers return the same bytes.
func Record(m proto.Message) ([]byte, error) {
	return proto.MarshalOptions{Deterministic: true}.Marshal(m)
}

func DecodeRecord[R proto.Message](b []byte) (R, error) {
	var zero R
	r := zero.ProtoReflect().New().Interface().(R)
	if err := proto.Unmarshal(b, r); err != nil {
		return zero, err
	}
	return r, nil
}

func EncodeRecords[R proto.Message](recs []R) ([]byte, error) {
	items := make([][]byte, len(recs))
	for i, r := range recs {
		var err error
		if items[i], err = Record(r); err != nil {
			return nil, err
		}
	}
	return EncodeList(items), nil
}

func DecodeRecords[R proto.Message](b []byte) ([]R, error) {
	items, err := DecodeList(b)
	if err != nil {
		return nil, err
	}
	recs := make([]R, len(items))
	for i, item := range items {
		if recs[i], err = DecodeRecord[R](item); err != nil {
			return nil, err
		}
	}
	return recs, nil
}

func EncodeEnrollUser(record *v1.UserSigned, publicKey, passwordHash string) ([][]byte, error) {
	rec, err := Record(record)
	if err != nil {
		return nil, err
	}
	return [][]byte{rec, []byte(publicKey), []byte(passwordHash)}, nil
}

func DecodeEnrollUser(args [][]byte) (record *v1.UserSigned, publicKey, passwordHash string, err error) {
	if err := arity(args, 3); err != nil {
		return nil, "", "", err
	}
	record = &v1.UserSigned{}
	if err := proto.Unmarshal(args[0], record); err != nil {
		return nil, "", "", store.Errf(store.InvalidContent, "wire", fmt.Errorf("record: %w", err))
	}
	return record, string(args[1]), string(args[2]), nil
}

func EncodeWrite(callerID string, record proto.Message) ([][]byte, error) {
	rec, err := Record(record)
	if err != nil {
		return nil, err
	}
	return [][]byte{[]byte(callerID), rec}, nil
}

func DecodeWrite(args [][]byte, record proto.Message) (callerID string, err error) {
	if err := arity(args, 2); err != nil {
		return "", err
	}
	if err := proto.Unmarshal(args[1], record); err != nil {
		return "", store.Errf(store.InvalidContent, "wire", fmt.Errorf("record: %w", err))
	}
	return string(args[0]), nil
}

func EncodeStrings(s ...string) [][]byte {
	out := make([][]byte, len(s))
	for i, v := range s {
		out[i] = []byte(v)
	}
	return out
}

func DecodeStrings(args [][]byte, n int) ([]string, error) {
	if err := arity(args, n); err != nil {
		return nil, err
	}
	out := make([]string, n)
	for i, a := range args {
		out[i] = string(a)
	}
	return out, nil
}

func arity(args [][]byte, n int) error {
	if len(args) != n {
		return store.Errf(store.InvalidContent, "wire", fmt.Errorf("%d arguments, want %d", len(args), n))
	}
	return nil
}

// EncodeList packs items as the bytes of a message `repeated bytes items = 1`.
func EncodeList(items [][]byte) []byte {
	var b []byte
	for _, it := range items {
		b = protowire.AppendTag(b, 1, protowire.BytesType)
		b = protowire.AppendBytes(b, it)
	}
	return b
}

func DecodeList(b []byte) ([][]byte, error) {
	var out [][]byte
	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return nil, fmt.Errorf("wire: list: %w", protowire.ParseError(n))
		}
		if num != 1 || typ != protowire.BytesType {
			return nil, fmt.Errorf("wire: list: field %d of type %d, want 1 bytes", num, typ)
		}
		b = b[n:]
		item, n := protowire.ConsumeBytes(b)
		if n < 0 {
			return nil, fmt.Errorf("wire: list: %w", protowire.ParseError(n))
		}
		out = append(out, item)
		b = b[n:]
	}
	return out, nil
}

// errorPrefix opens every failure message the chaincode returns, so the client
// can find the Kind inside whatever the peer and gateway wrap around it
// ("chaincode response 500, …"). The numeric status does not survive that
// wrapping reliably; the message does.
const errorPrefix = "metacensus:"

// ErrorMessage writes err's store.Kind, if any, where ParseKind finds it.
func ErrorMessage(err error) string {
	if kind := store.KindOf(err); kind != "" {
		return errorPrefix + string(kind) + ": " + err.Error()
	}
	return err.Error()
}

// ParseKind finds the Kind ErrorMessage wrote anywhere in msg, or "" if there
// is none.
func ParseKind(msg string) store.Kind {
	_, rest, ok := strings.Cut(msg, errorPrefix)
	if !ok {
		return ""
	}
	name, _, ok := strings.Cut(rest, ":")
	if !ok {
		return ""
	}
	return store.Kind(name)
}
