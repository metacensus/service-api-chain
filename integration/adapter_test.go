//go:build adapter

package integration

import (
	"context"
	"log"
	"net"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/hyperledger/fabric-chaincode-go/v2/shim"
	pb "github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"google.golang.org/grpc"

	"github.com/metacensus/api/go/signing"
	"github.com/metacensus/api/go/store"
	"github.com/metacensus/api/go/store/storetest"

	"github.com/metacensus/service-api-chain/internal/chaincode"
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
	ccPort := lis.Addr().(*net.TCPAddr).Port

	t0 := time.Now()
	f, err := startFabric(ctx, ccPort)
	if f != nil {
		defer f.close(ctx)
	}
	if err != nil {
		log.Printf("start Fabric: %v", err)
		return 1
	}
	log.Printf("Microfab started in %s", time.Since(t0).Round(time.Millisecond))

	t0 = time.Now()
	srv := grpc.NewServer()
	defer srv.Stop()
	name, err := deployInProcess(ctx, f, srv, lis)
	if err != nil {
		log.Printf("deploy: %v", err)
		return 1
	}
	log.Printf("chaincode deployed in %s", time.Since(t0).Round(time.Millisecond))

	st, closeStore, err := gateway.Connect(f.options(name))
	if err != nil {
		log.Printf("connect: %v", err)
		return 1
	}
	defer closeStore()
	sharedStore = st

	code := m.Run()
	if code != 0 {
		f.dumpLogs(ctx)
	}
	return code
}

func deployInProcess(ctx context.Context, f *fab, srv *grpc.Server, lis net.Listener) (string, error) {
	port := lis.Addr().(*net.TCPAddr).Port
	name := "inproc" + strconv.FormatInt(time.Now().Unix(), 10)
	pkg, pkgID, err := pack(name, net.JoinHostPort(hostAlias, strconv.Itoa(port)))
	if err != nil {
		return "", err
	}
	pb.RegisterChaincodeServer(srv, &shim.ChaincodeServer{
		CCID:     pkgID,
		Address:  lis.Addr().String(),
		CC:       chaincode.New(signing.ParticipantPolicy(storetest.Origin)),
		TLSProps: shim.TLSProperties{Disabled: true},
	})
	go srv.Serve(lis)
	return name, f.define(ctx, name, pkg, pkgID)
}
