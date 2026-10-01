package gateway

import "context"

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
