package config

import (
	"errors"
	"maps"
	"reflect"
	"strings"
	"testing"

	"github.com/metacensus/service-api-chain/internal/gateway"
)

func serviceEnv(override map[string]string) func(string) string {
	env := map[string]string{
		"FABRIC_PEER_ENDPOINT": "peer.example:7051",
		"FABRIC_PEER_TLS_CA":   "/certs/ca.pem",
		"FABRIC_MSP_ID":        "Org1MSP",
		"FABRIC_CERT":          "/id/cert.pem",
		"FABRIC_KEY":           "/id/key.pem",
		"FABRIC_CHANNEL":       "metacensus",
		"FABRIC_CHAINCODE":     "store",
	}
	maps.Copy(env, override)
	return func(key string) string { return env[key] }
}

func TestLoadService(t *testing.T) {
	tests := []struct {
		name     string
		override map[string]string
		want     Service
		problems []string
	}{
		{
			name: "success - defaults the port",
			want: Service{Port: defaultPort, Fabric: gateway.Options{
				PeerEndpoint: "peer.example:7051", PeerTLSCAFile: "/certs/ca.pem",
				MSPID: "Org1MSP", CertFile: "/id/cert.pem", KeyFile: "/id/key.pem",
				Channel: "metacensus", Chaincode: "store",
			}},
		},
		{
			name:     "success - explicit port, authority and trimmed values",
			override: map[string]string{"PORT": "8080", "FABRIC_PEER_AUTHORITY": "peer0.org1", "FABRIC_CHANNEL": " metacensus "},
			want: Service{Port: 8080, Fabric: gateway.Options{
				PeerEndpoint: "peer.example:7051", PeerTLSCAFile: "/certs/ca.pem", PeerAuthority: "peer0.org1",
				MSPID: "Org1MSP", CertFile: "/id/cert.pem", KeyFile: "/id/key.pem",
				Channel: "metacensus", Chaincode: "store",
			}},
		},
		{
			name:     "success - plaintext only when asked for",
			override: map[string]string{"FABRIC_PEER_TLS_CA": "", "FABRIC_PEER_PLAINTEXT": "1"},
			want: Service{Port: defaultPort, Fabric: gateway.Options{
				PeerEndpoint: "peer.example:7051",
				MSPID:        "Org1MSP", CertFile: "/id/cert.pem", KeyFile: "/id/key.pem",
				Channel: "metacensus", Chaincode: "store",
			}},
		},
		{
			name:     "error - no CA and no plaintext opt-in",
			override: map[string]string{"FABRIC_PEER_TLS_CA": ""},
			problems: []string{"FABRIC_PEER_TLS_CA is required"},
		},
		{
			name:     "error - CA and plaintext together",
			override: map[string]string{"FABRIC_PEER_PLAINTEXT": "1"},
			problems: []string{"contradict"},
		},
		{
			name:     "error - endpoint without a port",
			override: map[string]string{"FABRIC_PEER_ENDPOINT": "peer.example"},
			problems: []string{"FABRIC_PEER_ENDPOINT must be host:port"},
		},
		{
			name:     "error - bad port",
			override: map[string]string{"PORT": "70000"},
			problems: []string{"PORT must be a port"},
		},
		{
			name:     "error - unparseable port",
			override: map[string]string{"PORT": "no"},
			problems: []string{"PORT must be a port"},
		},
		{
			name: "error - everything missing is reported at once",
			override: map[string]string{
				"FABRIC_PEER_ENDPOINT": "", "FABRIC_PEER_TLS_CA": "", "FABRIC_MSP_ID": "",
				"FABRIC_CERT": "", "FABRIC_KEY": "", "FABRIC_CHANNEL": "", "FABRIC_CHAINCODE": "",
			},
			problems: []string{
				"FABRIC_PEER_ENDPOINT is required", "FABRIC_PEER_TLS_CA is required",
				"FABRIC_MSP_ID is required", "FABRIC_CERT is required", "FABRIC_KEY is required",
				"FABRIC_CHANNEL is required", "FABRIC_CHAINCODE is required",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := LoadService(serviceEnv(tt.override))
			if tt.problems == nil {
				if err != nil {
					t.Fatalf("LoadService: %v", err)
				}
				if !reflect.DeepEqual(got, tt.want) {
					t.Errorf("got %+v, want %+v", got, tt.want)
				}
				return
			}
			var probs problems
			if !errors.As(err, &probs) {
				t.Fatalf("err = %v, want problems", err)
			}
			if len(probs) != len(tt.problems) {
				t.Fatalf("problems = %v, want %d", probs, len(tt.problems))
			}
			for i, sub := range tt.problems {
				if !strings.Contains(probs[i], sub) {
					t.Errorf("problem %d = %q, want it to contain %q", i, probs[i], sub)
				}
			}
		})
	}
}
