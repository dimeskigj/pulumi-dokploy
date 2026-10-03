package dokploy

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
)

// lookupDecode reads only the fields declared by the lookup-specific type. The
// generated resource decoders must not inspect excluded upstream properties.
func lookupDecode[T any](operation string, response *http.Response, requestErr error) (T, error) {
	var value T
	if requestErr != nil {
		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}
		return value, lookupError(operation, requestErr)
	}
	if response == nil || response.Body == nil {
		return value, lookupResponseError(operation)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return value, lookupError(operation, &client.APIError{StatusCode: response.StatusCode})
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return value, lookupResponseError(operation)
	}
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return value, lookupResponseError(operation)
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	if err := decoder.Decode(&value); err != nil {
		return value, lookupResponseError(operation)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return value, lookupResponseError(operation)
	}
	return value, nil
}

type lookupNamed struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
}

type lookupProject struct {
	lookupNamed
	ProjectId            *string `json:"projectId"`
	DefaultEnvironmentId *string `json:"defaultEnvironmentId"`
}
type lookupEnvironment struct {
	lookupNamed
	EnvironmentId *string `json:"environmentId"`
	ProjectId     *string `json:"projectId"`
	IsDefault     *bool   `json:"isDefault"`
}
type lookupWorkload struct {
	lookupNamed
	AppName       *string `json:"appName"`
	EnvironmentId *string `json:"environmentId"`
	ServerId      *string `json:"serverId"`
}
type lookupApplication struct {
	lookupWorkload
	ApplicationId     *string `json:"applicationId"`
	ApplicationStatus *string `json:"applicationStatus"`
	RegistryId        *string `json:"registryId"`
	BuildRegistryId   *string `json:"buildRegistryId"`
}
type lookupCompose struct {
	lookupWorkload
	ComposeId     *string `json:"composeId"`
	ComposeStatus *string `json:"composeStatus"`
	ComposeType   *string `json:"composeType"`
}
type lookupDatabase struct {
	lookupWorkload
	DockerImage       *string `json:"dockerImage"`
	Image             *string `json:"image"`
	ApplicationStatus *string `json:"applicationStatus"`
	ExternalPort      *int    `json:"externalPort"`
}
type lookupPostgres struct {
	lookupDatabase
	PostgresId   *string `json:"postgresId"`
	DatabaseName *string `json:"databaseName"`
	DatabaseUser *string `json:"databaseUser"`
}
type lookupMysql struct {
	lookupDatabase
	MysqlId      *string `json:"mysqlId"`
	DatabaseName *string `json:"databaseName"`
	DatabaseUser *string `json:"databaseUser"`
}
type lookupMariadb struct {
	lookupDatabase
	MariadbId    *string `json:"mariadbId"`
	DatabaseName *string `json:"databaseName"`
	DatabaseUser *string `json:"databaseUser"`
}
type lookupMongo struct {
	lookupDatabase
	MongoId      *string `json:"mongoId"`
	DatabaseUser *string `json:"databaseUser"`
	ReplicaSets  *bool   `json:"replicaSets"`
}
type lookupRedis struct {
	lookupDatabase
	RedisId *string `json:"redisId"`
}
type lookupServer struct {
	lookupNamed
	ServerId       *string `json:"serverId"`
	IpAddress      *string `json:"ipAddress"`
	Port           *int    `json:"port"`
	Username       *string `json:"username"`
	OrganizationId *string `json:"organizationId"`
	SshKeyId       *string `json:"sshKeyId"`
	ServerType     *string `json:"serverType"`
	ServerStatus   *string `json:"serverStatus"`
}
type lookupRegistry struct {
	RegistryId   *string `json:"registryId"`
	RegistryName *string `json:"registryName"`
	RegistryUrl  *string `json:"registryUrl"`
	Username     *string `json:"username"`
	ImagePrefix  *string `json:"imagePrefix"`
	ServerId     *string `json:"serverId"`
	RegistryType *string `json:"registryType"`
}
type lookupSSHKey struct {
	lookupNamed
	SshKeyId       *string `json:"sshKeyId"`
	PublicKey      *string `json:"publicKey"`
	OrganizationId *string `json:"organizationId"`
}

func lookupImage(canonical, legacy *string) *string {
	if canonical != nil {
		return canonical
	}
	return legacy
}
