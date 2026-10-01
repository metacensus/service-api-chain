// Package fabrictest is what the suites share: a software passkey, and under the
// integration and artifact tags, Microfab.
package fabrictest

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"fmt"
	"testing"
	"time"

	v1 "github.com/metacensus/api/go/metacensus/v1"
	"github.com/metacensus/api/go/signing"
	"github.com/metacensus/api/go/store/storetest"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Person is a software passkey: a key that signs what a participant's
// authenticator would.
type Person struct {
	key       *ecdsa.PrivateKey
	KeyID     string
	PublicKey string // as SignUpRequest.public_key carries it
}

// NewPerson generates a fresh key.
func NewPerson(t *testing.T) Person {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate a key: %v", err)
	}
	keyID, err := signing.KeyID(&key.PublicKey)
	if err != nil {
		t.Fatalf("thumbprint the key: %v", err)
	}
	publicKey, err := signing.EncodePublicKey(&key.PublicKey)
	if err != nil {
		t.Fatalf("encode the key: %v", err)
	}
	return Person{key: key, KeyID: keyID, PublicKey: publicKey}
}

func (p Person) Sign(t *testing.T, origin string, content proto.Message) (*v1.Interpretation, *v1.Signature) {
	t.Helper()
	interp := signing.Interpretation(content)
	at := timestamppb.New(time.Unix(1_700_000_000, 0).UTC())
	challenge, err := signing.UserChallenge(content, interp, p.KeyID, at)
	if err != nil {
		t.Fatalf("compute the challenge: %v", err)
	}
	authData := signing.AuthenticatorData(storetest.RPID, signing.FlagUP|signing.FlagUV)
	assertion, err := signing.Assert(p.key, challenge, authData, signing.ClientData{Type: signing.TypeGet, Origin: origin})
	if err != nil {
		t.Fatalf("assert: %v", err)
	}
	return interp, &v1.Signature{KeyId: p.KeyID, Time: at, Assertion: assertion}
}

func (p Person) User(t *testing.T, origin, id, email string) *v1.UserSigned {
	t.Helper()
	content := &v1.User{Name: "Integration", Email: email, Country: "GB"}
	interp, sig := p.Sign(t, origin, content)
	return &v1.UserSigned{
		Id:             id,
		Recorded:       timestamppb.Now(),
		Content:        content,
		Interpretation: interp,
		UserSignature:  sig,
	}
}

// Unique is kind plus a random suffix, so tests sharing a ledger never collide.
func Unique(kind string) string { return fmt.Sprintf("%s-%s", kind, rand.Text()) }
