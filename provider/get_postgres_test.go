package dokploy

import "testing"

func TestLookupDatabasesPostgres(t *testing.T) {
	testDatabaseLookup(t, "postgres", "postgresId", func(factory clientFactory, id string) (any, error) {
		return (GetPostgres{client: factory}).Invoke(t.Context(), databaseRequest[GetPostgresArgs](GetPostgresArgs{PostgresID: id}))
	})
}
