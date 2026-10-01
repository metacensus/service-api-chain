//go:build artifact

package integration

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"google.golang.org/protobuf/proto"

	"github.com/metacensus/api/go/contract"
	v1 "github.com/metacensus/api/go/metacensus/v1"
	"github.com/metacensus/api/go/server/routes"
	"github.com/metacensus/api/go/store/storetest"
)

const (
	servicePort   = "3001/tcp"
	chaincodePort = 9999

	chaincodeAlias = "chaincode"

	appOrigin = "https://app.test"

	bootTimeout = 90 * time.Second

	// Where the service image reads the admin identity.
	containerCert = "/fabric/cert.pem"
	containerKey  = "/fabric/key.pem"
)

// image is the container request for an image under test: the one named by
// envVar when set (CI builds each once), else the Dockerfile built here.
func image(envVar, dockerfile string) testcontainers.ContainerRequest {
	if name := os.Getenv(envVar); name != "" {
		return testcontainers.ContainerRequest{Image: name}
	}
	return testcontainers.ContainerRequest{FromDockerfile: testcontainers.FromDockerfile{
		Context:    "..",
		Dockerfile: dockerfile,
		KeepImage:  true,
	}}
}

func serviceImage() testcontainers.ContainerRequest {
	req := image("SERVICE_IMAGE", "Dockerfile")
	req.ExposedPorts = []string{servicePort}
	return req
}

func chaincodeImage() testcontainers.ContainerRequest {
	return image("CHAINCODE_IMAGE", "Dockerfile.chaincode")
}

// start runs req, and on failure prints the container's logs before it is terminated.
func start(t *testing.T, ctx context.Context, req testcontainers.ContainerRequest) testcontainers.Container {
	t.Helper()
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if container != nil {
		t.Cleanup(func() {
			if t.Failed() {
				if logs, err := container.Logs(context.Background()); err == nil {
					body, _ := io.ReadAll(logs)
					t.Logf("container logs (%s):\n%s", req.Image, body)
				}
			}
			_ = container.Terminate(context.Background())
		})
	}
	if err != nil {
		t.Fatalf("start container: %v", err)
	}
	return container
}

func TestImages_RefuseToBoot(t *testing.T) {
	tests := []struct {
		name string
		req  testcontainers.ContainerRequest
		env  map[string]string
	}{
		{
			name: "error - service with no configuration",
			req:  serviceImage(),
			env:  map[string]string{},
		},
		{
			name: "error - chaincode with no configuration",
			req:  chaincodeImage(),
			env:  map[string]string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			tt.req.Env = tt.env
			tt.req.WaitingFor = wait.ForExit().WithExitTimeout(bootTimeout)
			container := start(t, ctx, tt.req)

			state, err := container.State(ctx)
			if err != nil {
				t.Fatalf("container state: %v", err)
			}
			if state.ExitCode == 0 {
				t.Errorf("exit code = 0, want non-zero")
			}
		})
	}
}

// TestImages_Serve deploys the chaincode image to Microfab, points the service
// image at it, and walks the API over HTTP: sign up, log in, then a topic, a
// prop and a vote, each persisted by a committed transaction.
func TestImages_Serve(t *testing.T) {
	ctx := context.Background()
	f := microfab
	t0 := time.Now()

	// The chaincode: packaged to be dialled by alias, and told its own package
	// id, which exists before the container does.
	name := "image" + strconv.FormatInt(time.Now().Unix(), 10)
	pkg, pkgID, err := pack(name, fmt.Sprintf("%s:%d", chaincodeAlias, chaincodePort))
	if err != nil {
		t.Fatalf("package the chaincode: %v", err)
	}
	cc := chaincodeImage()
	cc.Env = map[string]string{
		"CHAINCODE_ID":             pkgID,
		"CHAINCODE_SERVER_ADDRESS": fmt.Sprintf(":%d", chaincodePort),
		"ALLOWED_ORIGINS":          appOrigin,
		"CHAINCODE_PLAINTEXT":      "1",
	}
	cc.Networks = []string{f.net.Name}
	cc.NetworkAliases = map[string][]string{f.net.Name: {chaincodeAlias}}
	cc.WaitingFor = wait.ForLog(`"msg":"serving"`).WithStartupTimeout(bootTimeout)
	start(t, ctx, cc)
	if err := f.define(ctx, name, pkg, pkgID); err != nil {
		t.Fatalf("deploy the chaincode image: %v", err)
	}
	t.Logf("chaincode image deployed in %s", time.Since(t0).Round(time.Millisecond))

	svc := serviceImage()
	svc.Env = map[string]string{
		"FABRIC_PEER_ENDPOINT":  microfabAlias + ":2000",
		"FABRIC_PEER_PLAINTEXT": "1",
		"FABRIC_MSP_ID":         mspID,
		"FABRIC_CERT":           containerCert,
		"FABRIC_KEY":            containerKey,
		"FABRIC_CHANNEL":        channel,
		"FABRIC_CHAINCODE":      name,
	}
	svc.Files = []testcontainers.ContainerFile{
		{HostFilePath: f.certFile, ContainerFilePath: containerCert, FileMode: 0o644},
		{HostFilePath: f.keyFile, ContainerFilePath: containerKey, FileMode: 0o644},
	}
	svc.Networks = []string{f.net.Name}
	svc.WaitingFor = wait.ForHTTP("/healthz").WithPort(servicePort).WithStartupTimeout(bootTimeout)
	container := start(t, ctx, svc)

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("container host: %v", err)
	}
	port, err := container.MappedPort(ctx, servicePort)
	if err != nil {
		t.Fatalf("mapped port: %v", err)
	}
	api := &client{t: t, base: fmt.Sprintf("http://%s:%s", host, port.Port())}

	t.Run("error - the API refuses a caller with no session", func(t *testing.T) {
		status, _ := api.send(http.MethodGet, routes.Prefix+"/self", "", nil)
		if status != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", status)
		}
	})

	t.Run("success - sign up, log in, then a topic, a prop and a vote", func(t *testing.T) {
		t0 := time.Now()
		me := newPerson(t)
		email := unique("ada") + "@" + storetest.RPID
		const password = "correct horse battery staple"

		user := &v1.User{Name: "Ada", Email: email, Country: "GB"}
		interp, sig := me.sign(t, appOrigin, user)
		var signedUp v1.Session
		api.call(http.MethodPost, "/signup", "", &v1.SignUpRequest{
			Content: user, Password: password, Interpretation: interp, PublicKey: me.publicKey, UserSignature: sig,
		}, &signedUp)
		if signedUp.GetToken() == "" {
			t.Fatal("sign-up returned no access token")
		}

		var session v1.Session
		api.call(http.MethodPost, "/login", "", &v1.LoginRequest{Email: email, Password: password}, &session)
		token := session.GetToken()
		if token == "" {
			t.Fatal("login returned no access token")
		}

		var self v1.UserSigned
		api.call(http.MethodGet, "/self", token, nil, &self)
		if !proto.Equal(self.GetContent(), user) {
			t.Fatalf("self = %v, want %v", self.GetContent(), user)
		}

		topic := &v1.Topic{Name: "Elections", Description: "voting reform"}
		interp, sig = me.sign(t, appOrigin, topic)
		var topicRec v1.TopicSigned
		api.call(http.MethodPost, "/topic", token, &v1.TopicCreateRequest{
			Content: topic, Interpretation: interp, UserSignature: sig,
		}, &topicRec)
		topicID := topicRec.GetId()

		prop := &v1.Prop{TopicId: topicID, Type: v1.Prop_Statement, Description: "ranked choice"}
		interp, sig = me.sign(t, appOrigin, prop)
		var propRec v1.PropSigned
		api.call(http.MethodPost, "/topic/"+topicID+"/prop", token, &v1.PropCreateRequest{
			TopicId: topicID, Content: prop, Interpretation: interp, UserSignature: sig,
		}, &propRec)
		propID := propRec.GetId()

		votePath := "/topic/" + topicID + "/prop/" + propID + "/vote"
		vote := &v1.Vote{TopicId: topicID, PropId: propID, UserId: self.GetId(), Position: v1.Vote_For}
		interp, sig = me.sign(t, appOrigin, vote)
		api.call(http.MethodPost, votePath, token, &v1.VoteSetRequest{
			TopicId: topicID, PropId: propID, Content: vote, Interpretation: interp, UserSignature: sig,
		}, &v1.VoteSigned{})

		var votes v1.VoteList
		api.call(http.MethodGet, votePath, token, nil, &votes)
		if len(votes.GetItems()) != 1 {
			t.Fatalf("votes = %d, want 1", len(votes.GetItems()))
		}
		if got := votes.GetItems()[0].GetContent(); !proto.Equal(got, vote) {
			t.Errorf("vote = %v, want %v", got, vote)
		}
		t.Logf("flow took %s", time.Since(t0).Round(time.Millisecond))
	})
}

type client struct {
	t    *testing.T
	base string
}

func (c *client) send(method, path, token string, body []byte) (int, []byte) {
	c.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, bytes.NewReader(body))
	if err != nil {
		c.t.Fatalf("new request: %v", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	out, err := io.ReadAll(resp.Body)
	if err != nil {
		c.t.Fatalf("read %s %s: %v", method, path, err)
	}
	return resp.StatusCode, out
}

// call sends in as protojson, requires 200, and decodes the reply into out.
func (c *client) call(method, path, token string, in, out proto.Message) {
	c.t.Helper()
	var body []byte
	if in != nil {
		var err error
		if body, err = contract.Marshal(in); err != nil {
			c.t.Fatalf("marshal %s %s: %v", method, path, err)
		}
	}
	status, got := c.send(method, routes.Prefix+path, token, body)
	if status != http.StatusOK {
		c.t.Fatalf("%s %s: status %d, body %s", method, path, status, got)
	}
	if err := contract.Unmarshal(got, out); err != nil {
		c.t.Fatalf("decode %s %s: %v", method, path, err)
	}
}

var microfab *fab

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
	ctx := context.Background()
	t0 := time.Now()
	f, err := startFabric(ctx)
	if f != nil {
		defer f.close(ctx)
	}
	if err != nil {
		log.Printf("start Fabric: %v", err)
		return 1
	}
	log.Printf("Microfab started in %s", time.Since(t0).Round(time.Millisecond))
	microfab = f

	code := m.Run()
	if code != 0 {
		f.dumpLogs(ctx)
	}
	return code
}
