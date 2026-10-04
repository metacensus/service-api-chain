package gateway

type Options struct {
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

	// Channel holds users and topics; Chaincode is the store's chaincode as
	// defined there and on every topic channel.
	Channel   string
	Chaincode string
}
