package main

import (
	"errors"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

const (
	gatewayPkg       = "github.com/metacensus/service-api-chain/internal/gateway"
	fabricGatewayPkg = "github.com/hyperledger/fabric-gateway"

	chaincodePkg = "github.com/metacensus/service-api-chain/cmd/chaincode"
	servicePkg   = "github.com/metacensus/service-api-chain/cmd/service"
)

// TestForbiddenDeps holds the chaincode, which runs on the peers, to a graph
// without the API's gateway client. It reads `go list -deps`, what the linker
// uses, so an import anywhere below the binary counts.
func TestForbiddenDeps(t *testing.T) {
	tests := []struct {
		name string
		pkg  string
	}{
		{name: "success - the chaincode links no gateway client", pkg: chaincodePkg},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := forbiddenDeps(t, tt.pkg, gatewayPkg, fabricGatewayPkg); len(got) > 0 {
				t.Errorf("%s links forbidden package(s) %q", tt.pkg, got)
			}
		})
	}
}

// TestForbiddenDeps_Fires proves the guard sees what it forbids: the service
// binary dials the gateway, so it must report both.
func TestForbiddenDeps_Fires(t *testing.T) {
	tests := []struct {
		name string
		pkg  string
		want []string
	}{
		{name: "success - the service links both", pkg: servicePkg, want: []string{gatewayPkg, fabricGatewayPkg}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := forbiddenDeps(t, tt.pkg, gatewayPkg, fabricGatewayPkg); !slices.Equal(got, tt.want) {
				t.Errorf("%s links forbidden package(s) %q, want %q", tt.pkg, got, tt.want)
			}
		})
	}
}

// forbiddenDeps returns each of forbidden that pkg's transitive imports include,
// as the package itself or one beneath it, in the order given.
func forbiddenDeps(t *testing.T, pkg string, forbidden ...string) []string {
	t.Helper()
	out, err := exec.Command("go", "list", "-deps", "-f", "{{.ImportPath}}", pkg).Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			t.Fatalf("go list -deps %s: %v\n%s", pkg, err, exit.Stderr)
		}
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
