package config

import (
	"github.com/metacensus/service-api-chain/internal/gateway"
)

const defaultPort = 3001

type Service struct {
	Port   int
	Fabric gateway.Options
}

// LoadService never infers plaintext from the peer's address; FABRIC_PEER_PLAINTEXT=1 must ask for it.
func LoadService(getenv func(string) string) (Service, error) {
	r := &reader{getenv: getenv}

	s := Service{Port: r.port("PORT", defaultPort)}

	endpoint := r.hostPort("FABRIC_PEER_ENDPOINT")

	ca := r.optional("FABRIC_PEER_TLS_CA")
	plaintext := r.optional("FABRIC_PEER_PLAINTEXT") == "1"
	switch {
	case ca == "" && !plaintext:
		r.problemf("FABRIC_PEER_TLS_CA is required (set FABRIC_PEER_PLAINTEXT=1 to dial the peer unencrypted, for a test network only)")
	case ca != "" && plaintext:
		r.problemf("FABRIC_PEER_TLS_CA and FABRIC_PEER_PLAINTEXT=1 contradict each other")
	}

	s.Fabric = gateway.Options{
		PeerEndpoint:  endpoint,
		PeerTLSCAFile: ca,
		PeerAuthority: r.optional("FABRIC_PEER_AUTHORITY"),
		MSPID:         r.required("FABRIC_MSP_ID"),
		CertFile:      r.required("FABRIC_CERT"),
		KeyFile:       r.required("FABRIC_KEY"),
		Channel:       r.required("FABRIC_CHANNEL"),
		Chaincode:     r.required("FABRIC_CHAINCODE"),
	}

	if err := r.err(); err != nil {
		return Service{}, err
	}
	return s, nil
}
