package dokploy

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

// requireNoError is retained for legacy call sites; live failures use a
// structural diagnostic and never expose provider/API details.
func requireNoError(t *testing.T, err error, msgAndArgs ...interface{}) {
	t.Helper()
	if err != nil {
		resource, operation := "live resource", "operation"
		if len(msgAndArgs) > 0 {
			if value, ok := msgAndArgs[0].(string); ok && value != "" {
				resource = value
			}
		}
		if len(msgAndArgs) > 1 {
			if value, ok := msgAndArgs[1].(string); ok && value != "" {
				operation = value
			}
		}
		t.Fatalf("%s", structuralLiveError(resource, operation, err))
	}
}

func structuralLiveError(resource, operation string, _ error) string {
	return liveLifecycleDiagnostic(resource, operation, "", nil)
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

func requireLivePresent(t *testing.T, field string, value interface{}) {
	t.Helper()
	if value == nil || reflect.ValueOf(value).IsZero() {
		t.Errorf("live field %s was not present", field)
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
func liveProject(t *testing.T, ctx context.Context, api *client.Client) (id, defaultEnvironmentID string, release func()) {
	t.Helper()
	r := Project{client: fixedClient(api)}
	created, err := r.Create(ctx, infer.CreateRequest[ProjectArgs]{Inputs: ProjectArgs{Name: liveRunName("project")}})
	return finalizeLiveProjectCreate(t, api, created, err)
}

func finalizeLiveProjectCreate(t *testing.T, api *client.Client, created infer.CreateResponse[ProjectState], err error) (id, defaultEnvironmentID string, release func()) {
	t.Helper()
	r := Project{client: fixedClient(api)}
	release = func() {}
	var owner *liveCleanupOwner
	if created.ID != "" {
		owner = registerLiveCleanupOwner(t, "project", created.ID, func(ctx context.Context) error {
			_, err := r.Delete(ctx, infer.DeleteRequest[ProjectState]{ID: created.ID})
			return err
		}, func(ctx context.Context) (string, error) {
			read, err := r.Read(ctx, infer.ReadRequest[ProjectArgs, ProjectState]{ID: created.ID})
			return read.ID, err
		})
		release = owner.release
	}
	if created.ID == "" {
		if !liveCreateDefinitelyRejected(err) {
			recordLiveCleanupFailure("project", "", err)
		}
		if err == nil {
			err = fmt.Errorf("project create returned no confirmed identity")
		}
		requireNoError(t, err, "Project", "create")
	}
	if err == nil && created.Output.DefaultEnvironmentID == "" {
		err = fmt.Errorf("project create returned no confirmed default environment")
	}
	if err != nil && created.ID != "" {
		cleanupCtx, cancel := cleanupContext()
		cleanupErr := releaseAfterVerifiedCleanup(owner, func() error {
			return verifyLiveCleanup(cleanupCtx, func(ctx context.Context) error {
				_, cleanupErr := r.Delete(ctx, infer.DeleteRequest[ProjectState]{ID: created.ID})
				return cleanupErr
			}, func(ctx context.Context) (string, error) {
				read, readErr := r.Read(ctx, infer.ReadRequest[ProjectArgs, ProjectState]{ID: created.ID})
				return read.ID, readErr
			})
		})
		cancel()
		if cleanupErr == nil {
			release()
		} else {
			reportLiveCleanup(t, "project", created.ID, cleanupErr)
		}
	}
	requireNoError(t, err, "Project", "create")
	return created.ID, created.Output.DefaultEnvironmentID, release
}
