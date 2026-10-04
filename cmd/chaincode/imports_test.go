package main

import (
	"os/exec"
	"slices"
	"strings"
	"testing"
)

const (
	gatewayPkg       = "github.com/metacensus/service-api-chain/internal/gateway"
	fabricGatewayPkg = "github.com/hyperledger/fabric-gateway"
)

// TestForbiddenDeps holds the chaincode, which runs on the peers, to a graph
// without the API's gateway client. It reads `go list -deps`, what the linker
// uses, so an import anywhere below the binary counts. The service binary
// proves the guard fires: it is the one that dials the gateway.
func TestForbiddenDeps(t *testing.T) {
	tests := []struct {
		name string
		pkg  string
		want []string
	}{
		{
			name: "success - the chaincode links no gateway client",
			pkg:  "github.com/metacensus/service-api-chain/cmd/chaincode",
		},
		{
			name: "error - the service links both, so the guard fires",
			pkg:  "github.com/metacensus/service-api-chain/cmd/service",
			want: []string{gatewayPkg, fabricGatewayPkg},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := forbiddenDeps(t, tt.pkg, gatewayPkg, fabricGatewayPkg)
			if !slices.Equal(got, tt.want) {
				t.Errorf("%s depends on %q, want %q", tt.pkg, got, tt.want)
			}
		})
	}
}

// forbiddenDeps returns each of forbidden that pkg's transitive imports include,
// by package or by any package beneath it, in the order given.
func forbiddenDeps(t *testing.T, pkg string, forbidden ...string) []string {
	t.Helper()
	out, err := exec.Command("go", "list", "-deps", "-f", "{{.ImportPath}}", pkg).Output()
	if err != nil {
		t.Fatalf("go list -deps %s: %v", pkg, err)
	}
	deps := strings.Fields(string(out))
	var found []string
	for _, f := range forbidden {
		if slices.ContainsFunc(deps, func(d string) bool { return d == f || strings.HasPrefix(d, f+"/") }) {
			found = append(found, f)
		}
	}
	return found
}
