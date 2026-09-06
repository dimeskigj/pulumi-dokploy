package dokploy

import (
	"context"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

// requireNoError surfaces ResourceInitFailedError reasons without exposing
// unsanitized credentials in live-test diagnostics.
func requireNoError(t *testing.T, err error, msgAndArgs ...interface{}) {
	t.Helper()
	var initErr infer.ResourceInitFailedError
	if errors.As(err, &initErr) {
		t.Fatalf("%s (reasons: %s)", sanitizedLiveError(err, msgAndArgs...), sanitizeLiveDiagnostic(fmt.Sprint(initErr.Reasons)))
	}
	if err != nil {
		t.Fatalf("%s", sanitizedLiveError(err, msgAndArgs...))
	}
}

func liveClient(t *testing.T) *client.Client {
	t.Helper()
	requireLiveAcceptance(t)
	api, err := client.New(os.Getenv("DOKPLOY_ENDPOINT"), os.Getenv("DOKPLOY_API_KEY"))
	require.NoError(t, err)
	return api
}

func liveContext(t *testing.T, timeout time.Duration) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	t.Cleanup(cancel)
	return ctx
}

func liveRegistryArgs(t *testing.T) (RegistryArgs, bool) {
	t.Helper()
	missing := make([]string, 0, 3)
	for _, name := range []string{"DOKPLOY_REGISTRY_URL", "DOKPLOY_REGISTRY_USERNAME", "DOKPLOY_REGISTRY_PASSWORD"} {
		if os.Getenv(name) == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) != 0 {
		return RegistryArgs{}, false
	}
	var prefix *string
	if value := os.Getenv("DOKPLOY_REGISTRY_IMAGE_PREFIX"); value != "" {
		prefix = &value
	}
	return RegistryArgs{Name: liveRunName("registry"), URL: os.Getenv("DOKPLOY_REGISTRY_URL"), Username: os.Getenv("DOKPLOY_REGISTRY_USERNAME"), Password: os.Getenv("DOKPLOY_REGISTRY_PASSWORD"), ImagePrefix: prefix}, true
}

func liveSSHKeyPair(t *testing.T) (privateKey, publicKey string) {
	t.Helper()
	public, private, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	privateBytes, err := x509.MarshalPKCS8PrivateKey(private)
	require.NoError(t, err)
	publicKeyData, err := ssh.NewPublicKey(public)
	require.NoError(t, err)
	privateKey = string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateBytes}))
	publicKey = string(ssh.MarshalAuthorizedKey(publicKeyData))
	t.Cleanup(registerLiveSecrets(privateKey, publicKey))
	return privateKey, publicKey
}

func TestLiveSSHKeyPairUsesDokployKeyFormats(t *testing.T) {
	privateKey, publicKey := liveSSHKeyPair(t)
	parsedPublic, _, _, _, err := ssh.ParseAuthorizedKey([]byte(publicKey))
	require.NoError(t, err)
	require.Equal(t, ssh.KeyAlgoED25519, parsedPublic.Type())
	block, _ := pem.Decode([]byte(privateKey))
	require.NotNil(t, block)
	parsedPrivate, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	require.NoError(t, err)
	require.IsType(t, ed25519.PrivateKey{}, parsedPrivate)
}

// liveProject creates a scratch project and verifies its eventual absence.
func liveProject(t *testing.T, ctx context.Context, api *client.Client) (id, defaultEnvironmentID string) {
	t.Helper()
	r := Project{client: fixedClient(api)}
	created, err := r.Create(ctx, infer.CreateRequest[ProjectArgs]{Inputs: ProjectArgs{Name: liveRunName("project")}})
	if created.ID != "" {
		t.Cleanup(func() {
			liveCleanupVerified(t, "project", created.ID, func(ctx context.Context) error {
				_, err := r.Delete(ctx, infer.DeleteRequest[ProjectState]{ID: created.ID})
				return err
			}, func(ctx context.Context) (string, error) {
				read, err := r.Read(ctx, infer.ReadRequest[ProjectArgs, ProjectState]{ID: created.ID})
				return read.ID, err
			})
		})
	}
	if err != nil && created.ID != "" {
		liveCleanupVerified(t, "project", created.ID, func(ctx context.Context) error {
			_, cleanupErr := r.Delete(ctx, infer.DeleteRequest[ProjectState]{ID: created.ID})
			return cleanupErr
		}, func(ctx context.Context) (string, error) {
			read, readErr := r.Read(ctx, infer.ReadRequest[ProjectArgs, ProjectState]{ID: created.ID})
			return read.ID, readErr
		})
	}
	requireNoError(t, err)
	return created.ID, created.Output.DefaultEnvironmentID
}
