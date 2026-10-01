package gateway

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/hyperledger/fabric-gateway/pkg/client"
	"github.com/hyperledger/fabric-protos-go-apiv2/gateway"
	"github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/metacensus/api/go/store"

	"github.com/metacensus/service-api-chain/internal/wire"
)

var errOpaque = errors.New("opaque")

// client.EndorseError wraps exactly this status, but its fields are unexported.
func statusWithDetails(t *testing.T, code codes.Code, msg string, details ...string) error {
	t.Helper()
	s := status.New(code, msg)
	for _, d := range details {
		var err error
		if s, err = s.WithDetails(&gateway.ErrorDetail{Message: d}); err != nil {
			t.Fatal(err)
		}
	}
	return s.Err()
}

func TestClassify(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want store.Kind
	}{
		{
			name: "success - kind in an endorse detail",
			err: statusWithDetails(t, codes.Aborted, "failed to endorse transaction, see attached details for more info",
				"chaincode response 500, "+wire.ErrorMessage(store.Errf(store.AlreadyExists, "email taken", nil))),
			want: store.AlreadyExists,
		},
		{
			name: "success - kind in the second of two details",
			err:  statusWithDetails(t, codes.Aborted, "failed", "peer down", wire.ErrorMessage(store.Errf(store.NotFound, "no such topic", nil))),
			want: store.NotFound,
		},
		{
			name: "success - kind in an evaluate status message",
			err:  statusWithDetails(t, codes.Aborted, "evaluate: chaincode response 500, "+wire.ErrorMessage(store.Errf(store.SignatureInvalid, "bad", nil))),
			want: store.SignatureInvalid,
		},
		{
			name: "success - kind survives wrapping",
			err:  fmt.Errorf("outer: %w", statusWithDetails(t, codes.Aborted, wire.ErrorMessage(store.Errf(store.Unauthenticated, "x", nil)))),
			want: store.Unauthenticated,
		},
		{
			name: "success - chaincode kind outranks the transport code",
			err:  statusWithDetails(t, codes.Unavailable, wire.ErrorMessage(store.Errf(store.InvalidContent, "x", nil))),
			want: store.InvalidContent,
		},
		{
			name: "success - mvcc read conflict",
			err:  &client.CommitError{Code: peer.TxValidationCode_MVCC_READ_CONFLICT},
			want: store.Unavailable,
		},
		{
			name: "success - phantom read conflict",
			err:  fmt.Errorf("wrapped: %w", &client.CommitError{Code: peer.TxValidationCode_PHANTOM_READ_CONFLICT}),
			want: store.Unavailable,
		},
		{
			name: "success - submit error",
			err:  &client.SubmitError{TransactionError: &client.TransactionError{}},
			want: store.Unavailable,
		},
		{
			name: "success - commit status error",
			err:  &client.CommitStatusError{TransactionError: &client.TransactionError{}},
			want: store.Unavailable,
		},
		{
			name: "success - grpc unavailable",
			err:  statusWithDetails(t, codes.Unavailable, "connection refused"),
			want: store.Unavailable,
		},
		{
			name: "success - grpc deadline exceeded",
			err:  statusWithDetails(t, codes.DeadlineExceeded, "deadline"),
			want: store.Unavailable,
		},
		{
			name: "success - context canceled",
			err:  context.Canceled,
			want: store.Unavailable,
		},
		{
			name: "success - context deadline exceeded, wrapped",
			err:  fmt.Errorf("w: %w", context.DeadlineExceeded),
			want: store.Unavailable,
		},
		{
			name: "success - unrelated commit failure is unclassified",
			err:  &client.CommitError{Code: peer.TxValidationCode_BAD_PAYLOAD},
			want: "",
		},
		{
			name: "success - other grpc status is unclassified",
			err:  statusWithDetails(t, codes.Internal, "boom"),
			want: "",
		},
		{
			name: "success - plain error is unclassified",
			err:  errOpaque,
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classify(tt.err); got != tt.want {
				t.Errorf("classify = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFailure(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantKind store.Kind
	}{
		{name: "success - classified carries its kind", err: context.Canceled, wantKind: store.Unavailable},
		{name: "success - unclassified carries none", err: errOpaque},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := failure("CreateTopic", tt.err)
			assertKind(t, got, tt.wantKind)
			if !errors.Is(got, tt.err) {
				t.Errorf("failure does not wrap its cause: %v", got)
			}
		})
	}
}
