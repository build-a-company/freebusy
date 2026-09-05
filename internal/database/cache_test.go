package database

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/the-protobuf-project/runtime-go/cache"
	"github.com/the-protobuf-project/runtime-go/cache/redis"

	"github.com/oh-tarnished/freebusy/internal/database/cache/freebusy/promocodecache"
	"github.com/oh-tarnished/freebusy/internal/database/gorm/freebusy/promocode"
)

// redisAddr is where the dev overlay points; the docker container is `my-redis`.
const redisAddr = "127.0.0.1:6379"

// skipWithoutRedis keeps the suite green on a machine with no cache server. The
// test below asserts a capability of the backend, so there is nothing useful to
// assert without one — and failing would only report the absence of Redis.
func skipWithoutRedis(t *testing.T) {
	t.Helper()
	conn, err := net.DialTimeout("tcp", redisAddr, 500*time.Millisecond)
	if err != nil {
		t.Skipf("no redis at %s: %v", redisAddr, err)
	}
	_ = conn.Close()
}

// TestCachedPromoCodeStoreRequiresSets pins the capability contract that decides
// which backends may serve this schema.
//
// PromoCode is annotated STRATEGY_INDEXED, and an indexed resource needs
// server-side sets to maintain its secondary index. The generated constructor
// probes for that at construction rather than trusting configuration, so a
// backend without sets (memcached) fails here instead of returning a miss on
// every lookup by code — a failure that would otherwise appear only in the
// environment that runs memcached, and never on a developer's Redis.
//
// Constructing successfully is the assertion: it means the probe ran against a
// real server and found core.Sets.
func TestCachedPromoCodeStoreRequiresSets(t *testing.T) {
	skipWithoutRedis(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := redis.NewClient(ctx, redis.Config{Address: redisAddr})
	if err != nil {
		t.Fatalf("redis.NewClient: %v", err)
	}
	provider := redis.New(client, cache.Config{Prefix: "freebusy-test", RequireTTL: true})

	db, err := promocodecache.OpenPromoCodeCache(ctx, provider)
	if err != nil {
		t.Fatalf("OpenPromoCodeCache: %v", err)
	}

	// A nil *gorm.DB is enough for the inner store: the constructor only checks
	// that it is non-nil and then probes the cache backend. Nothing here reaches
	// Postgres, so this test needs no database.
	inner := promocode.NewPromoCodeStore(nil)
	if _, err := promocodecache.NewCachedPromoCodeStore(ctx, inner, db); err != nil {
		t.Fatalf("NewCachedPromoCodeStore against redis: %v", err)
	}
}

// TestOpenCacheProviderDisabled pins the documented "no cache configured" path:
// a nil Provider and no error, so a caller falls back to the undecorated store
// rather than treating an intentionally-off cache as a startup failure.
func TestOpenCacheProviderDisabled(t *testing.T) {
	// config.Get() reads the embedded release defaults, where cache.enabled is
	// false; the dev overlay that turns it on is not loaded in a unit test.
	p, err := OpenCacheProvider(context.Background())
	if err != nil {
		t.Fatalf("OpenCacheProvider with cache disabled: %v", err)
	}
	if p != nil {
		t.Fatalf("OpenCacheProvider = %v, want nil when disabled", p)
	}
}
