package wire

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	v1 "github.com/metacensus/api/go/metacensus/v1"
	"github.com/metacensus/api/go/store"
)

func topic() *v1.TopicSigned {
	return &v1.TopicSigned{
		Id:       "t1",
		Recorded: &timestamppb.Timestamp{Seconds: 1700000000, Nanos: 123456789},
		Content:  &v1.Topic{Name: "n", Description: "d"},
	}
}

func TestEnrollUser(t *testing.T) {
	user := &v1.UserSigned{Id: "u1", Content: &v1.User{Email: "a@b.c"}}
	good, err := EncodeEnrollUser(user, "pk", "hash")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		args    [][]byte
		wantErr bool
	}{
		{name: "success - round trip", args: good},
		{name: "error - too few arguments", args: good[:2], wantErr: true},
		{name: "error - record is not proto", args: [][]byte{{0xff}, good[1], good[2]}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec, pk, hash, err := DecodeEnrollUser(tt.args)
			if tt.wantErr {
				if got := store.KindOf(err); got != store.InvalidContent {
					t.Fatalf("err = %v (Kind %q); want Kind %q", err, got, store.InvalidContent)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !proto.Equal(rec, user) || pk != "pk" || hash != "hash" {
				t.Fatalf("got (%v, %q, %q); want (%v, pk, hash)", rec, pk, hash, user)
			}
		})
	}
}

func TestWrite(t *testing.T) {
	good, err := EncodeWrite("caller", topic())
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		args    [][]byte
		wantErr bool
	}{
		{name: "success - round trip keeps every timestamp nanosecond", args: good},
		{name: "error - record missing", args: good[:1], wantErr: true},
		{name: "error - record is not proto", args: [][]byte{good[0], {0xff}}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := &v1.TopicSigned{}
			caller, err := DecodeWrite(tt.args, got)
			if tt.wantErr {
				if got := store.KindOf(err); got != store.InvalidContent {
					t.Fatalf("err = %v (Kind %q); want Kind %q", err, got, store.InvalidContent)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if caller != "caller" || !proto.Equal(got, topic()) {
				t.Fatalf("got (%q, %v); want (caller, %v)", caller, got, topic())
			}
		})
	}
}

func TestDecodeStrings(t *testing.T) {
	tests := []struct {
		name    string
		args    [][]byte
		n       int
		want    []string
		wantErr bool
	}{
		{name: "success - two strings", args: EncodeStrings("t", "p"), n: 2, want: []string{"t", "p"}},
		{name: "success - no arguments", args: nil, n: 0, want: []string{}},
		{name: "error - wrong arity", args: EncodeStrings("t"), n: 2, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DecodeStrings(tt.args, tt.n)
			if tt.wantErr {
				if got := store.KindOf(err); got != store.InvalidContent {
					t.Fatalf("err = %v (Kind %q); want Kind %q", err, got, store.InvalidContent)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(got, tt.want) {
				t.Fatalf("got %q; want %q", got, tt.want)
			}
		})
	}
}

func TestList(t *testing.T) {
	tests := []struct {
		name    string
		in      []byte
		want    [][]byte
		wantErr bool
	}{
		{name: "success - empty list", in: EncodeList(nil), want: nil},
		{name: "success - items keep order and empty items survive", in: EncodeList([][]byte{[]byte("a"), {}, []byte("bc")}), want: [][]byte{[]byte("a"), {}, []byte("bc")}},
		{name: "error - truncated", in: EncodeList([][]byte{[]byte("abc")})[:3], wantErr: true},
		{name: "error - wrong field", in: []byte{0x10, 0x01}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DecodeList(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatal("decoded; want an error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !slices.EqualFunc(got, tt.want, bytes.Equal) {
				t.Fatalf("got %q; want %q", got, tt.want)
			}
		})
	}
}

func TestParseKind(t *testing.T) {
	tests := []struct {
		name string
		msg  string
		want store.Kind
	}{
		{name: "success - bare message", msg: ErrorMessage(store.Errf(store.NotFound, "GetUser", nil)), want: store.NotFound},
		{name: "success - wrapped by the peer", msg: "chaincode response 500, " + ErrorMessage(store.Errf(store.AlreadyExists, "EnrollUser", nil)), want: store.AlreadyExists},
		{name: "success - Kind wrapped before encoding", msg: ErrorMessage(fmt.Errorf("outer: %w", store.Errf(store.Unauthenticated, "SetVote", nil))), want: store.Unauthenticated},
		{name: "error - un-Kinded error", msg: "chaincode response 500, " + ErrorMessage(errors.New("state database unavailable"))},
		{name: "error - no prefix", msg: "chaincode response 500, boom"},
		{name: "error - prefix with no kind separator", msg: "metacensus:not_found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ParseKind(tt.msg); got != tt.want {
				t.Fatalf("ParseKind(%q) = %q; want %q", tt.msg, got, tt.want)
			}
		})
	}
}
