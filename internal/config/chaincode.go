package config

import (
	"net/url"
	"strings"
)

type Chaincode struct {
	ID      string
	Address string
	// AllowedOrigins are the WebAuthn origins a user signature may name.
	AllowedOrigins          []string
	TLSCertFile, TLSKeyFile string
	// TLSClientCAFile, when set, makes the server verify the peer (mutual TLS).
	TLSClientCAFile string
	Plaintext       bool
}

// LoadChaincode never infers plaintext; CHAINCODE_PLAINTEXT=1 must ask for it.
func LoadChaincode(getenv func(string) string) (Chaincode, error) {
	r := &reader{getenv: getenv}

	c := Chaincode{
		ID:              r.required("CHAINCODE_ID"),
		Address:         r.hostPort("CHAINCODE_SERVER_ADDRESS"),
		TLSCertFile:     r.optional("CHAINCODE_TLS_CERT"),
		TLSKeyFile:      r.optional("CHAINCODE_TLS_KEY"),
		TLSClientCAFile: r.optional("CHAINCODE_TLS_CLIENT_CA"),
		Plaintext:       r.optional("CHAINCODE_PLAINTEXT") == "1",
	}

	if raw := r.required("ALLOWED_ORIGINS"); raw != "" {
		for _, o := range strings.Split(raw, ",") {
			o = strings.TrimSpace(o)
			if !isOrigin(o) {
				r.problemf("ALLOWED_ORIGINS entry %q must be an origin, scheme://host[:port] with no path", o)
				continue
			}
			c.AllowedOrigins = append(c.AllowedOrigins, o)
		}
	}

	tls := c.TLSCertFile != "" || c.TLSKeyFile != ""
	switch {
	case tls && c.Plaintext:
		r.problemf("CHAINCODE_TLS_CERT/CHAINCODE_TLS_KEY and CHAINCODE_PLAINTEXT=1 contradict each other")
	case tls && (c.TLSCertFile == "" || c.TLSKeyFile == ""):
		r.problemf("CHAINCODE_TLS_CERT and CHAINCODE_TLS_KEY must be set together")
	case !tls && !c.Plaintext:
		r.problemf("CHAINCODE_TLS_CERT and CHAINCODE_TLS_KEY are required (set CHAINCODE_PLAINTEXT=1 to serve unencrypted, for a test network only)")
	case !tls && c.TLSClientCAFile != "":
		r.problemf("CHAINCODE_TLS_CLIENT_CA needs TLS, not CHAINCODE_PLAINTEXT=1")
	}

	if err := r.err(); err != nil {
		return Chaincode{}, err
	}
	return c, nil
}

func isOrigin(s string) bool {
	u, err := url.Parse(s)
	return err == nil && u.Scheme+"://"+u.Host == s && u.Hostname() != ""
}
