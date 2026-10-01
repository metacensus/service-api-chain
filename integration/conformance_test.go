//go:build adapter

package integration

import (
	"testing"

	"github.com/metacensus/api/go/store"
	"github.com/metacensus/api/go/store/storetest"
)

func TestConformance(t *testing.T) {
	storetest.Run(t, storetest.Harness{
		Open:       func(t *testing.T) store.Store { return sharedStore },
		Signatures: storetest.Hard,
	})
}
