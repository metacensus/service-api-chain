package chaincode

import (
	"errors"
	"fmt"
	"testing"

	"github.com/metacensus/api/go/store"

	"github.com/metacensus/service-api-chain/internal/wire"
)

func TestErrorMessage(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantKind store.Kind
	}{
		{name: "Kinded", err: store.Errf(store.NotFound, "GetUser", nil), wantKind: store.NotFound},
		{name: "Kinded then wrapped", err: fmt.Errorf("outer: %w", store.Errf(store.AlreadyExists, "EnrollUser", nil)), wantKind: store.AlreadyExists},
		{name: "un-Kinded", err: errors.New("state database unavailable")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := ErrorMessage(tt.err)
			if got := wire.ParseKind("chaincode response 500, " + msg); got != tt.wantKind {
				t.Errorf("ParseKind = %q, want %q (message %q)", got, tt.wantKind, msg)
			}
		})
	}
}
