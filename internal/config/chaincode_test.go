package config

import (
	"testing"

	"github.com/metacensus/service-api-chain/internal/chaincode"
)

var chaincodeBase = map[string]string{
	"CHAINCODE_ID":             "store:abc",
	"CHAINCODE_SERVER_ADDRESS": ":9999",
	"ALLOWED_ORIGINS":          "https://metacensus.example",
	"CHAINCODE_TLS_CERT":       "/tls/cert.pem",
	"CHAINCODE_TLS_KEY":        "/tls/key.pem",
}

func TestLoadChaincode(t *testing.T) {
	tests := []struct {
		name     string
		override map[string]string
		want     Chaincode
		problems []string
	}{
		{
			name: "success - TLS",
			want: Chaincode{ID: "store:abc", Address: ":9999", AllowedOrigins: []string{"https://metacensus.example"},
				TLSCertFile: "/tls/cert.pem", TLSKeyFile: "/tls/key.pem"},
		},
		{
			name:     "success - mutual TLS and several trimmed origins",
			override: map[string]string{"CHAINCODE_TLS_CLIENT_CA": "/tls/ca.pem", "ALLOWED_ORIGINS": "https://a.example, http://localhost:5173"},
			want: Chaincode{ID: "store:abc", Address: ":9999", AllowedOrigins: []string{"https://a.example", "http://localhost:5173"},
				TLSCertFile: "/tls/cert.pem", TLSKeyFile: "/tls/key.pem", TLSClientCAFile: "/tls/ca.pem"},
		},
		{
			name:     "success - explicit plaintext",
			override: map[string]string{"CHAINCODE_TLS_CERT": "", "CHAINCODE_TLS_KEY": "", "CHAINCODE_PLAINTEXT": "1"},
			want:     Chaincode{ID: "store:abc", Address: ":9999", AllowedOrigins: []string{"https://metacensus.example"}, Plaintext: true},
		},
		{
			name:     "error - every required variable missing",
			override: map[string]string{"CHAINCODE_ID": "", "CHAINCODE_SERVER_ADDRESS": "", "ALLOWED_ORIGINS": "", "CHAINCODE_TLS_CERT": "", "CHAINCODE_TLS_KEY": ""},
			problems: []string{"CHAINCODE_ID is required", "CHAINCODE_SERVER_ADDRESS is required", "ALLOWED_ORIGINS is required", "CHAINCODE_TLS_CERT and CHAINCODE_TLS_KEY are required"},
		},
		{
			name:     "error - address without a port",
			override: map[string]string{"CHAINCODE_SERVER_ADDRESS": "localhost"},
			problems: []string{"CHAINCODE_SERVER_ADDRESS must be host:port"},
		},
		{
			name:     "error - one bad origin among good",
			override: map[string]string{"ALLOWED_ORIGINS": "https://a.example,nonsense"},
			problems: []string{`"nonsense" must be an origin`},
		},
		{
			name:     "error - empty entry in the list",
			override: map[string]string{"ALLOWED_ORIGINS": "https://a.example,"},
			problems: []string{"must be an origin"},
		},
		{
			name:     "error - certificate without a key",
			override: map[string]string{"CHAINCODE_TLS_KEY": ""},
			problems: []string{"must be set together"},
		},
		{
			name:     "error - TLS and plaintext contradict",
			override: map[string]string{"CHAINCODE_PLAINTEXT": "1"},
			problems: []string{"contradict each other"},
		},
		{
			name:     "error - neither TLS nor plaintext",
			override: map[string]string{"CHAINCODE_TLS_CERT": "", "CHAINCODE_TLS_KEY": ""},
			problems: []string{"CHAINCODE_PLAINTEXT=1"},
		},
		{
			name:     "error - plaintext other than 1 is not an opt-in",
			override: map[string]string{"CHAINCODE_TLS_CERT": "", "CHAINCODE_TLS_KEY": "", "CHAINCODE_PLAINTEXT": "true"},
			problems: []string{"CHAINCODE_PLAINTEXT=1"},
		},
		{
			name:     "error - client CA without TLS",
			override: map[string]string{"CHAINCODE_TLS_CERT": "", "CHAINCODE_TLS_KEY": "", "CHAINCODE_PLAINTEXT": "1", "CHAINCODE_TLS_CLIENT_CA": "/tls/ca.pem"},
			problems: []string{"CHAINCODE_TLS_CLIENT_CA needs TLS"},
		},
		{
			name:     "success - the global channel and the chaincode's name there",
			override: map[string]string{"GLOBAL_CHANNEL": "metacensus", "GLOBAL_CHAINCODE": "store"},
			want: Chaincode{ID: "store:abc", Address: ":9999", AllowedOrigins: []string{"https://metacensus.example"},
				TLSCertFile: "/tls/cert.pem", TLSKeyFile: "/tls/key.pem", Global: chaincode.Config{GlobalChannel: "metacensus", GlobalChaincode: "store"}},
		},
		{
			name:     "error - the global channel without the chaincode's name",
			override: map[string]string{"GLOBAL_CHANNEL": "metacensus"},
			problems: []string{"GLOBAL_CHANNEL and GLOBAL_CHAINCODE must be set together"},
		},
		{
			name:     "error - the chaincode's name without the global channel",
			override: map[string]string{"GLOBAL_CHAINCODE": "store"},
			problems: []string{"GLOBAL_CHANNEL and GLOBAL_CHAINCODE must be set together"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := LoadChaincode(env(chaincodeBase, tt.override))
			assertLoad(t, got, err, tt.want, tt.problems)
		})
	}
}

func TestIsOriginEdges(t *testing.T) {
	for s, want := range map[string]bool{
		"https://a.example": true, "http://localhost:5173": true, "https://[::1]:8080": true,
		"https://a.example/": false, "https://a.example?": false, "https://a.example#": false,
		"https://u@a.example": false, "https://a.example/app": false, "a.example": false, "": false,
		"https://:443": false, "mailto:x@y": false, "HTTPS://a.example": false,
	} {
		if got := isOrigin(s); got != want {
			t.Errorf("isOrigin(%q) = %v, want %v", s, got, want)
		}
	}
}
