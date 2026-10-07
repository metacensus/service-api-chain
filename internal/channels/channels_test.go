package channels

import (
	"context"
	"errors"
	"testing"

	"github.com/metacensus/api/go/store"
)

func TestPerTopic_Resolve(t *testing.T) {
	tests := []struct {
		name    string
		topicID string
		want    string
	}{
		{name: "success - a topic id", topicID: "topc:0192a642-817d-7a3e-a282-d7a282ebd482", want: "metacensus.topic.0192a642-817d-7a3e-a282-d7a282ebd482"},
		{name: "error - another kind", topicID: "user:0192a642-817d-7a3e-a282-d7a282ebd482"},
		{name: "error - uppercase uuid", topicID: "topc:0192A642-817D-7A3E-A282-D7A282EBD482"},
		{name: "error - not a uuid", topicID: "topc:elections"},
		{name: "error - empty", topicID: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := PerTopic(nil).Resolve(tt.topicID)
			if tt.want == "" {
				if !errors.Is(err, store.InvalidContent) {
					t.Fatalf("err = %v, want InvalidContent", err)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("got (%q, %v), want %q", got, err, tt.want)
			}
		})
	}
}

func TestShared(t *testing.T) {
	s := Shared("metacensus")
	for _, id := range []string{"topc:0192a642-817d-7a3e-a282-d7a282ebd482", "anything", ""} {
		if got, err := s.Resolve(id); err != nil || got != "metacensus" {
			t.Errorf("Resolve(%q) = (%q, %v), want metacensus", id, got, err)
		}
		if err := s.Provision(t.Context(), id); err != nil {
			t.Errorf("Provision(%q) = %v", id, err)
		}
	}
}

func TestPerTopic(t *testing.T) {
	const topic = "topc:0192a642-817d-7a3e-a282-d7a282ebd482"
	const channel = "metacensus.topic.0192a642-817d-7a3e-a282-d7a282ebd482"

	t.Run("success - Provision creates the resolved channel", func(t *testing.T) {
		var created []string
		p := PerTopic(func(_ context.Context, ch string) error { created = append(created, ch); return nil })
		if err := p.Provision(t.Context(), topic); err != nil {
			t.Fatal(err)
		}
		got, err := p.Resolve(topic)
		if err != nil || got != channel {
			t.Fatalf("Resolve = (%q, %v), want %q", got, err, channel)
		}
		if len(created) != 1 || created[0] != channel {
			t.Errorf("created %q, want [%q]", created, channel)
		}
	})

	t.Run("error - a bad id is refused before create runs", func(t *testing.T) {
		p := PerTopic(func(context.Context, string) error { t.Fatal("create ran"); return nil })
		if err := p.Provision(t.Context(), "elections"); !errors.Is(err, store.InvalidContent) {
			t.Fatalf("err = %v, want InvalidContent", err)
		}
	})

	t.Run("error - create's failure is returned", func(t *testing.T) {
		boom := errors.New("orderer down")
		p := PerTopic(func(context.Context, string) error { return boom })
		if err := p.Provision(t.Context(), topic); !errors.Is(err, boom) {
			t.Fatalf("err = %v, want %v", err, boom)
		}
	})
}
