//go:build adapter

package integration

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/metacensus/api/go/store"
	"github.com/metacensus/api/go/store/storetest"
)

// One email, five concurrent enrolments: one wins, and at least one loser surfaces Fabric's MVCC conflict as Unavailable.
func TestStore_Conflict(t *testing.T) {
	st := sharedStore
	const contenders = 5
	email := unique("email") + "@" + storetest.RPID

	errs := make([]error, contenders)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range errs {
		p := newPerson(t)
		user := p.user(t, storetest.Origin, unique("user"), email)
		wg.Go(func() {
			<-start
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			errs[i] = st.EnrollUser(ctx, user, p.publicKey, "opaque-hash")
		})
	}
	close(start)
	wg.Wait()

	var won, conflicted int
	for _, err := range errs {
		switch store.KindOf(err) {
		case "":
			won++
		case store.Unavailable:
			conflicted++
		case store.AlreadyExists:
		default:
			t.Errorf("a contender failed with an unexpected error: %v", err)
		}
	}
	if won != 1 {
		t.Errorf("%d contenders won, want exactly 1: %v", won, errs)
	}
	if conflicted == 0 {
		t.Errorf("no contender lost to a commit-time conflict; every loser saw the email already taken: %v", errs)
	}
}
