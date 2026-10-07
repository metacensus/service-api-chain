//go:build integration || artifact

package fabrictest

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	fabcc "github.com/hyperledger/fabric-admin-sdk/pkg/chaincode"
	"github.com/hyperledger/fabric-admin-sdk/pkg/identity"
	"github.com/hyperledger/fabric-config/configtx"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/metacensus/service-api-chain/internal/config"
)

const (
	Channel = "metacensus"
	MSPID   = "Org1MSP"

	// Microfab's HTTP API, and the peer's own gRPC port. The peer is dialled
	// directly: Microfab's authority-routed proxy on 8080 intermittently drops
	// the trailers of an error response.
	microfabPort = "8080/tcp"
	peerPort     = "2000/tcp"
	// The orderer's own gRPC port (ORDERER_GENERAL_LISTENPORT inside the
	// container, not listed by its components API; 2004 is its operations
	// port). Dialled directly for the same reason as the peer.
	ordererPort = "2003/tcp"

	// HostAlias is how a container reaches this process's HostAccessPorts.
	HostAlias = "host.testcontainers.internal"
	// PeerAddr is how a container on Fab.Network reaches the peer.
	PeerAddr      = microfabAlias + ":2000"
	microfabAlias = "microfab"

	microfabConfig = `{"endorsing_organizations":[{"name":"Org1"}],"channels":[{"name":"metacensus","endorsing_organizations":["Org1"]}],"couchdb":false,"certificate_authorities":false}`
)

// Fab is the one network every test in a binary shares.
type Fab struct {
	Network *testcontainers.DockerNetwork
	// CertFile and KeyFile are Org1's admin identity.
	CertFile  string
	KeyFile   string
	container testcontainers.Container
	peerAddr  string // host:port of the peer itself, from this process
	admin     identity.SigningIdentity
	signer    *configtx.SigningIdentity // the same admin, as fabric-config signs with it
	peer      *grpc.ClientConn
	orderer   *grpc.ClientConn
	adminDir  string
}

// Start starts Microfab on a fresh network; containers reach hostPorts of this process at HostAlias.
// A failure tears down whatever started.
func Start(ctx context.Context, hostPorts ...int) (*Fab, error) {
	f, err := start(ctx, hostPorts)
	if err != nil {
		if f != nil {
			f.DumpLogs(ctx)
			f.Close(ctx)
		}
		return nil, err
	}
	return f, nil
}

func start(ctx context.Context, hostPorts []int) (*Fab, error) {
	nw, err := network.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("network: %w", err)
	}
	f := &Fab{Network: nw}

	f.container, err = testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:           "ghcr.io/hyperledger-labs/microfab:latest",
			ExposedPorts:    []string{microfabPort, peerPort, ordererPort},
			Env:             map[string]string{"MICROFAB_CONFIG": microfabConfig},
			Networks:        []string{nw.Name},
			NetworkAliases:  map[string][]string{nw.Name: {microfabAlias}},
			HostAccessPorts: hostPorts,
			WaitingFor:      wait.ForLog("Microfab started").WithStartupTimeout(3 * time.Minute),
		},
		Started: true,
	})
	if err != nil {
		return f, fmt.Errorf("microfab: %w", err)
	}

	host, err := f.container.Host(ctx)
	if err != nil {
		return f, err
	}
	port, err := f.container.MappedPort(ctx, microfabPort)
	if err != nil {
		return f, err
	}
	addr := net.JoinHostPort(host, port.Port())
	peer, err := f.container.MappedPort(ctx, peerPort)
	if err != nil {
		return f, err
	}
	f.peerAddr = net.JoinHostPort(host, peer.Port())
	orderer, err := f.container.MappedPort(ctx, ordererPort)
	if err != nil {
		return f, err
	}
	ordererAddr := net.JoinHostPort(host, orderer.Port())

	dir, err := os.MkdirTemp("", "microfab-admin-")
	if err != nil {
		return f, err
	}
	f.adminDir = dir
	f.CertFile, f.KeyFile = filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
	if err := f.fetchAdmin(addr); err != nil {
		return f, fmt.Errorf("admin identity: %w", err)
	}

	f.peer, err = grpc.NewClient(f.peerAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return f, err
	}
	f.orderer, err = grpc.NewClient(ordererAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return f, err
	}
	return f, nil
}

func (f *Fab) fetchAdmin(addr string) error {
	resp, err := http.Get("http://" + addr + "/ak/api/v1/components")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var components []struct {
		ID         string `json:"id"`
		MSPID      string `json:"msp_id"`
		Cert       string `json:"cert"`
		PrivateKey string `json:"private_key"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&components); err != nil {
		return err
	}
	for _, c := range components {
		if c.ID != "org1admin" {
			continue
		}
		cert, err := base64.StdEncoding.DecodeString(c.Cert)
		if err != nil {
			return err
		}
		key, err := base64.StdEncoding.DecodeString(c.PrivateKey)
		if err != nil {
			return err
		}
		if err := os.WriteFile(f.CertFile, cert, 0o600); err != nil {
			return err
		}
		if err := os.WriteFile(f.KeyFile, key, 0o600); err != nil {
			return err
		}
		x509Cert, err := identity.ReadCertificate(f.CertFile)
		if err != nil {
			return err
		}
		priv, err := identity.ReadPrivateKey(f.KeyFile)
		if err != nil {
			return err
		}
		f.signer = &configtx.SigningIdentity{Certificate: x509Cert, PrivateKey: priv, MSPID: c.MSPID}
		f.admin, err = identity.NewPrivateKeySigningIdentity(c.MSPID, x509Cert, priv)
		return err
	}
	return fmt.Errorf("no org1admin among %d components", len(components))
}

// Options connects the gateway to the peer, as the admin, at chaincodeName.
func (f *Fab) Options(chaincodeName string) config.Fabric {
	return config.Fabric{
		PeerEndpoint: f.peerAddr,
		MSPID:        MSPID,
		CertFile:     f.CertFile,
		KeyFile:      f.KeyFile,
		Global:       config.Global{Channel: Channel, Chaincode: chaincodeName},
	}
}

// Pack is a chaincode-as-a-service package whose connection.json points the peer at address.
func Pack(label, address string) (pkg []byte, pkgID string, err error) {
	dir, err := os.MkdirTemp("", "ccaas-")
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	err = fabcc.PackageCCAAS(
		fabcc.Connection{Address: address, DialTimeout: "10s"},
		fabcc.Metadata{Type: "ccaas", Label: label}, dir, "pkg.tgz")
	if err != nil {
		return nil, "", err
	}
	if pkg, err = os.ReadFile(filepath.Join(dir, "pkg.tgz")); err != nil {
		return nil, "", err
	}
	pkgID, err = fabcc.PackageID(bytes.NewReader(pkg))
	return pkg, pkgID, err
}

// Define installs pkg on the peer and defines it as name on Channel.
func (f *Fab) Define(ctx context.Context, name string, pkg []byte, pkgID string) error {
	if _, err := fabcc.NewPeer(f.peer, f.admin).Install(ctx, bytes.NewReader(pkg)); err != nil {
		return fmt.Errorf("install: %w", err)
	}
	return f.define(ctx, Channel, name, pkgID)
}

// define treats a definition already committed on channel as success.
func (f *Fab) define(ctx context.Context, channel, name, pkgID string) error {
	gw := fabcc.NewGateway(f.peer, f.admin)
	def := &fabcc.Definition{ChannelName: channel, PackageID: pkgID, Name: name, Version: "1", Sequence: 1}
	if err := gw.Approve(ctx, def); err != nil && !already(err, "redefine the current committed sequence") {
		return fmt.Errorf("approve: %w", err)
	}
	if err := gw.Commit(ctx, def); err != nil && !already(err, "new definition must be sequence 2") {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

func (f *Fab) DumpLogs(ctx context.Context) {
	if f.container == nil {
		return
	}
	if logs, err := f.container.Logs(ctx); err == nil {
		body, _ := io.ReadAll(logs)
		log.Printf("microfab logs:\n%s", body)
	}
}

// Run is a TestMain body: m.Run, then Microfab's logs if it failed, then Close.
func (f *Fab) Run(m *testing.M) int {
	ctx := context.Background()
	defer f.Close(ctx)
	code := m.Run()
	if code != 0 {
		f.DumpLogs(ctx)
	}
	return code
}

func (f *Fab) Close(ctx context.Context) {
	_ = os.RemoveAll(f.adminDir)
	if f.peer != nil {
		_ = f.peer.Close()
	}
	if f.orderer != nil {
		_ = f.orderer.Close()
	}
	if f.container != nil {
		_ = f.container.Terminate(ctx)
	}
	if f.Network != nil {
		_ = f.Network.Remove(ctx)
	}
}
