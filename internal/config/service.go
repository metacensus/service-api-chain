package config

const defaultPort = 3001

type Service struct {
	Port   int
	Fabric Fabric
}

// Fabric is how the API reaches its store: the peer to dial, the identity to
// submit as, and the chaincode to invoke. It lives here, not in gateway, so
// that reading the chaincode's configuration does not link the gateway client.
type Fabric struct {
	// PeerEndpoint is the peer gateway's gRPC address, host:port.
	PeerEndpoint string

	// PeerTLSCAFile is the path to the PEM CA that signed the peer's TLS certificate.
	// Empty dials plaintext.
	PeerTLSCAFile string

	// PeerAuthority, if set, replaces PeerEndpoint's host as gRPC authority and TLS server name.
	PeerAuthority string

	MSPID    string
	CertFile string
	KeyFile  string

	Channel   string
	Chaincode string
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

	s.Fabric = Fabric{
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
