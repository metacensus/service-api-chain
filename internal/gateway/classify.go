package gateway

import (
	"context"
	"errors"
	"strings"

	"github.com/hyperledger/fabric-gateway/pkg/client"
	"github.com/hyperledger/fabric-protos-go-apiv2/gateway"
	"github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/metacensus/api/go/store"

	"github.com/metacensus/service-api-chain/internal/wire"
)

func classify(err error) store.Kind {
	if kind, ok := chaincodeKind(err); ok {
		return kind
	}

	var commit *client.CommitError
	if errors.As(err, &commit) {
		switch commit.Code {
		case peer.TxValidationCode_MVCC_READ_CONFLICT, peer.TxValidationCode_PHANTOM_READ_CONFLICT:
			return store.Unavailable
		}
	}

	var submit *client.SubmitError
	var commitStatus *client.CommitStatusError
	if errors.As(err, &submit) || errors.As(err, &commitStatus) ||
		errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return store.Unavailable
	}

	if s, ok := status.FromError(err); ok {
		switch s.Code() {
		case codes.Unavailable, codes.DeadlineExceeded, codes.Canceled:
			return store.Unavailable
		}
	}
	return ""
}

// missingChannel reports the gateway refusing to invoke on a channel the peer
// is not serving the chaincode on: for an evaluation, the peer has no config
// for the channel; for a submission, discovery finds no endorser with the
// chaincode's metadata there. Both are the peer's own words, pinned by the
// integration suite.
func missingChannel(err error) bool {
	s, ok := status.FromError(err)
	if !ok {
		return false
	}
	switch s.Code() {
	case codes.Unavailable:
		return strings.Contains(s.Message(), "could not get last config for channel")
	case codes.FailedPrecondition:
		return strings.Contains(s.Message(), "No metadata was found for chaincode")
	}
	return false
}

// An EndorseError is a status error whose details carry each peer's chaincode message.
func chaincodeKind(err error) (store.Kind, bool) {
	s, ok := status.FromError(err)
	if !ok {
		return "", false
	}
	if kind := wire.ParseKind(s.Message()); kind != "" {
		return kind, true
	}
	for _, d := range s.Details() {
		if detail, ok := d.(*gateway.ErrorDetail); ok {
			if kind := wire.ParseKind(detail.GetMessage()); kind != "" {
				return kind, true
			}
		}
	}
	return "", false
}
