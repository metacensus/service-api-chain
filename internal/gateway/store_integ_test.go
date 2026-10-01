//go:build integration

package gateway_test

import (
	"context"
	"log"
	"net"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/hyperledger/fabric-chaincode-go/v2/shim"
	pb "github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"google.golang.org/grpc"

	"github.com/metacensus/api/go/signing"
	"github.com/metacensus/api/go/store"
	"github.com/metacensus/api/go/store/storetest"

	"github.com/metacensus/service-api-chain/internal/chaincode"
	"github.com/metacensus/service-api-chain/internal/fabrictest"
	"github.com/metacensus/service-api-chain/internal/gateway"
)

var sharedStore store.Store

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
	ctx := context.Background()

	// The port is chosen first: the container is told which host port to reach.
	lis, err := net.Listen("tcp", ":0")
	if err != nil {
		log.Printf("listen for the chaincode server: %v", err)
		return 1
	}
	port := lis.Addr().(*net.TCPAddr).Port

	f, err := fabrictest.Start(ctx, port)
	if err != nil {
		log.Printf("start Fabric: %v", err)
		return 1
	}

	srv := grpc.NewServer()
	defer srv.Stop()
	name, err := deployInProcess(ctx, f, srv, lis, port)
	if err != nil {
		log.Printf("deploy: %v", err)
		f.Close(ctx)
		return 1
	}
	st, closeStore, err := gateway.Connect(f.Options(name))
	if err != nil {
		log.Printf("connect: %v", err)
		f.Close(ctx)
		return 1
	}
	defer func() { _ = closeStore() }()
	sharedStore = st

	return f.Run(m)
}

func deployInProcess(ctx context.Context, f *fabrictest.Fab, srv *grpc.Server, lis net.Listener, port int) (string, error) {
	name := fabrictest.Unique("inproc")
	pkg, pkgID, err := fabrictest.Pack(name, net.JoinHostPort(fabrictest.HostAlias, strconv.Itoa(port)))
	if err != nil {
		return "", err
	}
	pb.RegisterChaincodeServer(srv, &shim.ChaincodeServer{
		CCID: pkgID,
		CC:   chaincode.New(signing.ParticipantPolicy(storetest.Origin)),
	})
	go func() { _ = srv.Serve(lis) }()
	return name, f.Define(ctx, name, pkg, pkgID)
}

func TestStore_Conformance(t *testing.T) {
	storetest.Run(t, storetest.Harness{
		Open:       func(t *testing.T) store.Store { return sharedStore },
		Signatures: storetest.Hard,
	})
}

// One email, five concurrent enrolments: one wins, and at least one loser surfaces Fabric's MVCC conflict as Unavailable.
func TestStore_Conflict(t *testing.T) {
	const contenders = 5
	email := fabrictest.Unique("email") + "@" + storetest.RPID

	errs := make([]error, contenders)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range errs {
		p := fabrictest.NewPerson(t)
		user := p.User(t, storetest.Origin, fabrictest.Unique("user"), email)
		wg.Go(func() {
			<-start
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			errs[i] = sharedStore.EnrollUser(ctx, user, p.PublicKey, "opaque-hash")
		})
	}
	close(start)
	wg.Wait()

	var won, conflicted int
	for _, err := range errs {
		switch {
		case err == nil:
			won++
		case store.KindOf(err) == store.Unavailable:
			conflicted++
		case store.KindOf(err) == store.AlreadyExists:
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
