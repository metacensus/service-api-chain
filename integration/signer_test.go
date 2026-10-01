//go:build adapter || artifact

package integration

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

// person is a software passkey: a key that signs what a participant's
// authenticator would.
type person struct {
	key       *ecdsa.PrivateKey
	keyID     string
	publicKey string // as SignUpRequest.public_key carries it
}

func newPerson(t *testing.T) person {
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
	return person{key: key, keyID: keyID, publicKey: publicKey}
}

func (p person) sign(t *testing.T, origin string, content proto.Message) (*v1.Interpretation, *v1.Signature) {
	t.Helper()
	interp := signing.Interpretation(content)
	at := timestamppb.New(time.Unix(1_700_000_000, 0).UTC())
	challenge, err := signing.UserChallenge(content, interp, p.keyID, at)
	if err != nil {
		t.Fatalf("compute the challenge: %v", err)
	}
	authData := signing.AuthenticatorData(storetest.RPID, signing.FlagUP|signing.FlagUV)
	assertion, err := signing.Assert(p.key, challenge, authData, signing.ClientData{Type: signing.TypeGet, Origin: origin})
	if err != nil {
		t.Fatalf("assert: %v", err)
	}
	return interp, &v1.Signature{KeyId: p.keyID, Time: at, Assertion: assertion}
}

func (p person) user(t *testing.T, origin, id, email string) *v1.UserSigned {
	t.Helper()
	content := &v1.User{Name: "Integration", Email: email, Country: "GB"}
	interp, sig := p.sign(t, origin, content)
	return &v1.UserSigned{
		Id:             id,
		Recorded:       timestamppb.Now(),
		Content:        content,
		Interpretation: interp,
		UserSignature:  sig,
	}
}

func unique(kind string) string { return fmt.Sprintf("%s-%s", kind, rand.Text()) }
