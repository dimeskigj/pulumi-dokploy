package dokploy

import "testing"

func TestLookupDatabasesMongoDB(t *testing.T) {
	testDatabaseLookup(t, "mongo", "mongoId", func(factory clientFactory, id string) (any, error) {
		return (GetMongoDB{client: factory}).Invoke(t.Context(), databaseRequest[GetMongoDBArgs](GetMongoDBArgs{MongoID: id}))
	})
}
