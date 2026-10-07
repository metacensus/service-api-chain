package channels

import (
	"errors"
	"testing"

	"github.com/metacensus/api/go/store"
)

func TestTopic(t *testing.T) {
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
			got, err := Topic(tt.topicID)
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
