package dokploy

import "testing"

func TestLookupDatabasesMySQL(t *testing.T) {
	testDatabaseLookup(t, "mysql", "mysqlId", func(factory clientFactory, id string) (any, error) {
		return (GetMySQL{client: factory}).Invoke(t.Context(), databaseRequest[GetMySQLArgs](GetMySQLArgs{MySQLID: id}))
	})
}
