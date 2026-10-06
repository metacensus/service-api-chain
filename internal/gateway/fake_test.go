package gateway

import (
	"context"

	"github.com/metacensus/api/go/store"

	"github.com/metacensus/service-api-chain/internal/channels"
)

type call struct {
	submit bool
	name   string
	args   [][]byte
}

type fakeContract struct {
	calls []call
	out   []byte
	err   error
}

func (f *fakeContract) Submit(_ context.Context, name string, args [][]byte) ([]byte, error) {
	f.calls = append(f.calls, call{submit: true, name: name, args: args})
	return f.out, f.err
}

func (f *fakeContract) Evaluate(_ context.Context, name string, args [][]byte) ([]byte, error) {
	f.calls = append(f.calls, call{name: name, args: args})
	return f.out, f.err
}

const globalChannel = "metacensus"

func sharedStore(f *fakeContract) store.Store {
	return newStore(func(string) invoker { return f }, globalChannel, channels.Shared(globalChannel))
}

type fakeNetwork struct {
	contracts map[string]*fakeContract
	asked     []string
}

func (n *fakeNetwork) contract(channel string) invoker {
	n.asked = append(n.asked, channel)
	if c, ok := n.contracts[channel]; ok {
		return c
	}
	c := &fakeContract{}
	if n.contracts == nil {
		n.contracts = map[string]*fakeContract{}
	}
	n.contracts[channel] = c
	return c
}
