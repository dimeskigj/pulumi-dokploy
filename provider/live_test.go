package dokploy

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
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

// Lifecycle diagnostics intentionally identify only structure. IDs, paths,
// SQL, and file contents must never be copied into acceptance output.
func liveLifecycleDiagnostic(resource, operation, field string, _ interface{}) string {
	if field == "" {
		return fmt.Sprintf("%s %s failed", resource, operation)
	}
	return fmt.Sprintf("%s %s failed for field %s", resource, operation, field)
}

func requireLiveLifecycleNoError(t *testing.T, resource, operation string, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s", liveLifecycleDiagnostic(resource, operation, "", err))
	}
}

func readReadyMountTarget(read func() (string, error), create func()) error {
	status, err := read()
	if err != nil {
		return fmt.Errorf("mount target read failed")
	}
	if status != statusDone {
		return fmt.Errorf("mount target is not ready")
	}
	create()
	return nil
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
	private, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	privateBytes := x509.MarshalPKCS1PrivateKey(private)
	publicKeyData, err := ssh.NewPublicKey(&private.PublicKey)
	require.NoError(t, err)
	privateKey = string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: privateBytes}))
	publicKey = string(ssh.MarshalAuthorizedKey(publicKeyData))
	t.Cleanup(registerLiveSecrets(privateKey, publicKey))
	return privateKey, publicKey
}

func TestLiveSSHKeyPairUsesDokployKeyFormats(t *testing.T) {
	privateKey, publicKey := liveSSHKeyPair(t)
	parsedPublic, _, _, _, err := ssh.ParseAuthorizedKey([]byte(publicKey))
	require.NoError(t, err)
	require.Equal(t, ssh.KeyAlgoRSA, parsedPublic.Type())
	block, _ := pem.Decode([]byte(privateKey))
	require.NotNil(t, block)
	require.Equal(t, "RSA PRIVATE KEY", block.Type)
	parsedPrivate, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	require.NoError(t, err)
	parsedPrivatePublic, err := ssh.NewPublicKey(&parsedPrivate.PublicKey)
	require.NoError(t, err)
	require.Equal(t, ssh.MarshalAuthorizedKey(parsedPrivatePublic), []byte(publicKey))
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
