package dokploy

import (
	"encoding/json"
	"testing"

	"github.com/dimeskigj/pulumi-dokploy/internal/client/generated"
	"github.com/stretchr/testify/require"
)

func TestLookupLegacyStatusCompatibility(t *testing.T) {
	readers := []struct {
		name string
		read func([]byte) (string, error)
	}{
		{"compose", func(raw []byte) (string, error) {
			var v generated.Compose
			if err := json.Unmarshal(raw, &v); err != nil {
				return "", err
			}
			return composeStatusValue(&v)
		}},
		{"postgres", func(raw []byte) (string, error) {
			var v generated.Postgres
			if err := json.Unmarshal(raw, &v); err != nil {
				return "", err
			}
			return postgresStatusValue(&v)
		}},
		{"mysql", func(raw []byte) (string, error) {
			var v generated.MySQL
			if err := json.Unmarshal(raw, &v); err != nil {
				return "", err
			}
			return mysqlStatusValue(&v)
		}},
		{"mariadb", func(raw []byte) (string, error) {
			var v generated.MariaDB
			if err := json.Unmarshal(raw, &v); err != nil {
				return "", err
			}
			return mariadbStatusValue(&v)
		}},
		{"mongodb", func(raw []byte) (string, error) {
			var v generated.MongoDB
			if err := json.Unmarshal(raw, &v); err != nil {
				return "", err
			}
			return mongodbStatusValue(&v)
		}},
		{"redis", func(raw []byte) (string, error) {
			var v generated.Redis
			if err := json.Unmarshal(raw, &v); err != nil {
				return "", err
			}
			return redisStatusValue(&v)
		}},
	}
	for _, reader := range readers {
		t.Run(reader.name, func(t *testing.T) {
			for _, tc := range []struct {
				name, raw, want string
				wantErr         string
			}{
				{"valid", `{"applicationStatus":"done","composeStatus":"done"}`, "done", ""},
				{"future", `{"applicationStatus":"waiting-new-state","composeStatus":"waiting-new-state"}`, "waiting-new-state", ""},
				{"missing", `{}`, "", "without a status"},
				{"empty", `{"applicationStatus":"","composeStatus":""}`, "", "invalid status"},
				{"wrong type", `{"applicationStatus":42,"composeStatus":42}`, "", "invalid status 42"},
			} {
				t.Run(tc.name, func(t *testing.T) {
					got, err := reader.read([]byte(tc.raw))
					if tc.wantErr != "" {
						require.ErrorContains(t, err, tc.wantErr)
						return
					}
					require.NoError(t, err)
					require.Equal(t, tc.want, got)
				})
			}
		})
	}
}
