package config

import (
	"errors"
	"maps"
	"reflect"
	"strings"
	"testing"
)

func env(base, override map[string]string) func(string) string {
	m := maps.Clone(base)
	maps.Copy(m, override)
	return func(key string) string { return m[key] }
}

func assertLoad[C any](t *testing.T, got C, err error, want C, wantProblems []string) {
	t.Helper()
	if wantProblems == nil {
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %+v, want %+v", got, want)
		}
		return
	}
	var probs problems
	if !errors.As(err, &probs) {
		t.Fatalf("err = %v, want problems", err)
	}
	if len(probs) != len(wantProblems) {
		t.Fatalf("problems = %q, want %d", probs, len(wantProblems))
	}
	for i, sub := range wantProblems {
		if !strings.Contains(probs[i], sub) {
			t.Errorf("problem %d = %q, want it to contain %q", i, probs[i], sub)
		}
	}
}
