package dokploy

import "testing"

func TestLookupDatabasesRedis(t *testing.T) {
	testDatabaseLookup(t, "redis", "redisId", func(factory clientFactory, id string) (any, error) {
		return (GetRedis{client: factory}).Invoke(t.Context(), databaseRequest[GetRedisArgs](GetRedisArgs{RedisID: id}))
	})
}
