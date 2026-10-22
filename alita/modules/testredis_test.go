//go:build testtools

package modules

import (
	"testing"

	"github.com/alicebob/miniredis/v2"
	gocache "github.com/eko/gocache/lib/v4/cache"
	"github.com/eko/gocache/lib/v4/marshaler"
	gocache_store "github.com/eko/gocache/store/redis/v4"
	"github.com/redis/go-redis/v9"

	"github.com/divkix/Alita_Robot/alita/utils/cache"
)

// withMiniredisServer starts an in-process Redis server (miniredis), installs a
// *redis.Client pointing at it via cache.SetCacheState, and stops the server when t
// finishes. It returns the server, so a test can reach what a client cannot, such as
// fast-forwarding Redis time to expire a key. Tests that exercise Redis-dependent paths
// call this instead of t.Skip so they run without a live Redis server.
func withMiniredisServer(t *testing.T) *miniredis.Miniredis {
	t.Helper()

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run() error = %v", err)
	}
	t.Cleanup(mr.Close)

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	t.Cleanup(func() { _ = client.Close() })

	previousMarshal, previousManager, previousClient := cache.GetCacheState()
	manager := gocache.New[any](gocache_store.NewRedis(client))
	cache.SetCacheState(marshaler.New(manager), manager, client)
	t.Cleanup(func() {
		cache.SetCacheState(previousMarshal, previousManager, previousClient)
	})
	return mr
}

// withMiniredis is withMiniredisServer for a test that never touches the server itself.
func withMiniredis(t *testing.T) {
	t.Helper()
	withMiniredisServer(t)
}
