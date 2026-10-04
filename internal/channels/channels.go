// Package channels places a topic's props and votes on a Fabric channel and
// brings that channel into being. It is the seam between the gateway, which
// routes every store call through it, and whatever runs the network: Microfab
// in the test suites, infra's Temporal workflows in production.
package channels

import (
	"context"

	"github.com/metacensus/api/go/store"
)

type Channels interface {
	// Resolve names the channel holding topicID's props and votes. It is
	// static, no lookup, so it fails only on an id that is not a topic id
	// (InvalidContent).
	Resolve(topicID string) (string, error)

	// Provision brings Resolve(topicID) into being: created on the orderer,
	// joined by the peers, the chaincode defined on it. It is not a chaincode
	// transaction and may fail partway; a re-run with the same topicID must
	// finish the job, so an existing channel, join or definition is success.
	// It returns once the chaincode answers on the channel.
	Provision(ctx context.Context, topicID string) error
}

// Shared is every topic on the one named channel: the backend's first shape,
// and the production default until a provisioner runs there.
type Shared string

func (s Shared) Resolve(string) (string, error)        { return string(s), nil }
func (Shared) Provision(context.Context, string) error { return nil }

// Prefix opens every per-topic channel name; the rest is the topic id's UUID,
// infra's convention (core/chaincode/contracts/topic.go). Channel names allow
// only lowercase letters, digits, '.' and '-', which a canonical UUID is.
const Prefix = "metacensus.topic."

// Name is the per-topic channel of topicID, or InvalidContent.
func Name(topicID string) (string, error) {
	u, err := store.ParseID(store.TopicID, topicID)
	if err != nil {
		return "", err
	}
	return Prefix + u.String(), nil
}

// PerTopic is a channel per topic, named by Name and brought into being by
// create, which is given the channel name and held to Provision's contract.
func PerTopic(create func(ctx context.Context, channel string) error) Channels {
	return perTopic{create: create}
}

type perTopic struct {
	create func(ctx context.Context, channel string) error
}

func (perTopic) Resolve(topicID string) (string, error) { return Name(topicID) }

func (p perTopic) Provision(ctx context.Context, topicID string) error {
	channel, err := Name(topicID)
	if err != nil {
		return err
	}
	return p.create(ctx, channel)
}
