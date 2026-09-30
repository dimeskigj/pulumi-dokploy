package dokploy

import "testing"

func TestLookupDatabasesMariaDB(t *testing.T) {
	testDatabaseLookup(t, "mariadb", "mariadbId", func(factory clientFactory, id string) (any, error) {
		return (GetMariaDB{client: factory}).Invoke(t.Context(), databaseRequest[GetMariaDBArgs](GetMariaDBArgs{MariaDBID: id}))
	})
}
