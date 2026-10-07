// Package channels names each Fabric channel with no lookup, as infra does
// (core/chaincode/contracts), and is the seam that brings a topic's channel into being.
package channels

import (
	"context"

	"github.com/metacensus/api/go/store"
)

// The global channels. Users also holds emails and keys; orgs is written by
// nothing yet.
const (
	Users  = "metacensus.users"
	Topics = "metacensus.topics"
	orgs   = "metacensus.orgs"
)

var Global = []string{Users, Topics, orgs}

// Topic names the channel holding topicID's props and votes; a canonical UUID
// is a valid channel name.
func Topic(topicID string) (string, error) {
	u, err := store.ParseID(store.TopicID, topicID)
	if err != nil {
		return "", err
	}
	return "metacensus.topic." + u.String(), nil
}

// Provision brings a topic's channel into being — the channel, the peers'
// join, the chaincode's definition — and returns once the chaincode answers
// there. A re-run must finish a partial one, so work already done is success.
type Provision func(ctx context.Context, channel string) error
