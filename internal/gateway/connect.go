package gateway

import (
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/hyperledger/fabric-gateway/pkg/client"
	"github.com/hyperledger/fabric-gateway/pkg/identity"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/metacensus/api/go/store"

	"github.com/metacensus/service-api-chain/internal/channels"
)

// Connect dials the peer named by o and returns the store over its chaincode,
// each topic's channel named by topics, and a close func that releases the
// gateway and then the connection.
func Connect(o Options, topics channels.Channels) (store.Store, func() error, error) {
	id, sign, err := loadIdentity(o)
	if err != nil {
		return nil, nil, err
	}
	conn, err := dial(o)
	if err != nil {
		return nil, nil, err
	}
	gw, err := client.Connect(id,
		client.WithSign(sign),
		client.WithClientConnection(conn),
		client.WithEvaluateTimeout(5*time.Second),
		client.WithEndorseTimeout(15*time.Second),
		client.WithSubmitTimeout(5*time.Second),
		client.WithCommitStatusTimeout(time.Minute),
	)
	if err != nil {
		return nil, nil, errors.Join(fmt.Errorf("gateway: connect: %w", err), conn.Close())
	}

	closeAll := func() error { return errors.Join(gw.Close(), conn.Close()) }
	contract := func(channel string) invoker {
		return contractInvoker{gw.GetNetwork(channel).GetContract(o.Chaincode)}
	}
	return newStore(contract, o.Channel, topics), closeAll, nil
}

func loadIdentity(o Options) (*identity.X509Identity, identity.Sign, error) {
	certPEM, err := os.ReadFile(o.CertFile)
	if err != nil {
		return nil, nil, fmt.Errorf("gateway: read cert: %w", err)
	}
	cert, err := identity.CertificateFromPEM(certPEM)
	if err != nil {
		return nil, nil, fmt.Errorf("gateway: cert %s: %w", o.CertFile, err)
	}
	id, err := identity.NewX509Identity(o.MSPID, cert)
	if err != nil {
		return nil, nil, fmt.Errorf("gateway: identity: %w", err)
	}

	keyPEM, err := os.ReadFile(o.KeyFile)
	if err != nil {
		return nil, nil, fmt.Errorf("gateway: read key: %w", err)
	}
	key, err := identity.PrivateKeyFromPEM(keyPEM)
	if err != nil {
		return nil, nil, fmt.Errorf("gateway: key %s: %w", o.KeyFile, err)
	}
	sign, err := identity.NewPrivateKeySign(key)
	if err != nil {
		return nil, nil, fmt.Errorf("gateway: signer: %w", err)
	}
	return id, sign, nil
}

func dial(o Options) (*grpc.ClientConn, error) {
	creds := insecure.NewCredentials()
	if o.PeerTLSCAFile != "" {
		caPEM, err := os.ReadFile(o.PeerTLSCAFile)
		if err != nil {
			return nil, fmt.Errorf("gateway: read peer CA: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caPEM) {
			return nil, fmt.Errorf("gateway: peer CA %s holds no certificate", o.PeerTLSCAFile)
		}
		creds = credentials.NewClientTLSFromCert(pool, o.PeerAuthority)
	}
	opts := []grpc.DialOption{grpc.WithTransportCredentials(creds)}
	if o.PeerAuthority != "" {
		opts = append(opts, grpc.WithAuthority(o.PeerAuthority))
	}
	conn, err := grpc.NewClient(o.PeerEndpoint, opts...)
	if err != nil {
		return nil, fmt.Errorf("gateway: dial %s: %w", o.PeerEndpoint, err)
	}
	return conn, nil
}
