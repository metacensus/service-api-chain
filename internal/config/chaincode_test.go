package config

import (
	"errors"
	"maps"
	"reflect"
	"strings"
	"testing"
)

func chaincodeEnv(override map[string]string) func(string) string {
	env := map[string]string{
		"CHAINCODE_ID":             "store:abc",
		"CHAINCODE_SERVER_ADDRESS": ":9999",
		"ALLOWED_ORIGINS":          "https://metacensus.example",
		"CHAINCODE_TLS_CERT":       "/tls/cert.pem",
		"CHAINCODE_TLS_KEY":        "/tls/key.pem",
	}
	maps.Copy(env, override)
	return func(key string) string { return env[key] }
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
			name:     "error - origin with a path",
			override: map[string]string{"ALLOWED_ORIGINS": "https://a.example/app"},
			problems: []string{"must be an origin"},
		},
		{
			name:     "error - origin with a trailing slash",
			override: map[string]string{"ALLOWED_ORIGINS": "https://a.example/"},
			problems: []string{"must be an origin"},
		},
		{
			name:     "error - origin without a scheme",
			override: map[string]string{"ALLOWED_ORIGINS": "a.example"},
			problems: []string{"must be an origin"},
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := LoadChaincode(chaincodeEnv(tt.override))
			if len(tt.problems) == 0 {
				if err != nil {
					t.Fatalf("LoadChaincode: %v", err)
				}
				if !reflect.DeepEqual(got, tt.want) {
					t.Errorf("got %+v, want %+v", got, tt.want)
				}
				return
			}
			var cfgErr *Error
			if !errors.As(err, &cfgErr) {
				t.Fatalf("err = %v, want *Error", err)
			}
			if len(cfgErr.Problems) != len(tt.problems) {
				t.Fatalf("problems = %q, want %d", cfgErr.Problems, len(tt.problems))
			}
			for i, want := range tt.problems {
				if !strings.Contains(cfgErr.Problems[i], want) {
					t.Errorf("problem %d = %q, want it to contain %q", i, cfgErr.Problems[i], want)
				}
			}
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
