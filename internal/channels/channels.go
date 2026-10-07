// Package channels names each topic's Fabric channel and brings it into being.
package channels

import (
	"context"

	"github.com/metacensus/api/go/store"
)

type Channels interface {
	// Resolve names topicID's channel without a lookup, failing
	// (InvalidContent) only on an id that is not a topic id.
	Resolve(topicID string) (string, error)

	// Provision brings Resolve(topicID) into being — the channel, the peers'
	// join, the chaincode's definition — and returns once the chaincode
	// answers there. A re-run must finish a partial one, so work already done
	// is success.
	Provision(ctx context.Context, topicID string) error
}

// Shared puts every topic on the one named channel, which already exists.
type Shared string

func (s Shared) Resolve(string) (string, error)        { return string(s), nil }
func (Shared) Provision(context.Context, string) error { return nil }

// PerTopic is a channel per topic, brought into being by calling it with the
// channel's name.
type PerTopic func(ctx context.Context, channel string) error

// Resolve follows infra's convention (core/chaincode/contracts/topic.go); a
// canonical UUID is a valid channel name.
func (PerTopic) Resolve(topicID string) (string, error) {
	u, err := store.ParseID(store.TopicID, topicID)
	if err != nil {
		return "", err
	}
	return "metacensus.topic." + u.String(), nil
}

func (create PerTopic) Provision(ctx context.Context, topicID string) error {
	channel, err := create.Resolve(topicID)
	if err != nil {
		return err
	}
	return create(ctx, channel)
}
