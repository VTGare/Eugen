package testutil

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/VTGare/Eugen/store"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TBF is the minimal subset of testing.TB / ginkgo.FullGinkgoTInterface
// that the test helpers need.
type TBF interface {
	Helper()
	Fatalf(format string, args ...any)
}

var dbCounter atomic.Uint32

// MongoContainer wraps a running MongoDB testcontainers instance and the
// connection URI needed to build *store.Store instances.
type MongoContainer struct {
	tc  testcontainers.Container
	uri string
}

// StartMongoDB starts a MongoDB container using testcontainers-go. The
// container is terminated when Cleanup is called. This is intended to be
// called once per test suite, typically in a Ginkgo BeforeSuite block.
func StartMongoDB(t TBF) *MongoContainer {
	t.Helper()

	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        "mongo:7.0",
		ExposedPorts: []string{"27017/tcp"},
		Env: map[string]string{
			"MONGO_INITDB_DATABASE": "test_eugen",
		},
		WaitingFor: wait.ForLog("Waiting for connections").WithStartupTimeout(60 * time.Second),
	}

	tc, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("failed to start MongoDB container: %v", err)
	}

	endpoint, err := tc.Endpoint(ctx, "")
	if err != nil {
		_ = tc.Terminate(ctx)
		t.Fatalf("failed to get container endpoint: %v", err)
	}

	return &MongoContainer{
		tc:  tc,
		uri: fmt.Sprintf("mongodb://%s", endpoint),
	}
}

// NewTestStore creates a *store.Store connected to the given MongoDB container.
// Each call creates a uniquely-named database (test_eugen_<n>) to provide
// isolation between tests within the same suite.
func (m *MongoContainer) NewTestStore(t TBF) *store.Store {
	t.Helper()

	ctx := context.Background()
	dbName := fmt.Sprintf("test_eugen_%d", dbCounter.Add(1))

	st, err := store.New(ctx, store.Config{
		URI:      m.uri,
		Database: dbName,
	})
	if err != nil {
		t.Fatalf("failed to connect to MongoDB: %v", err)
	}

	return st
}

// Cleanup terminates the MongoDB container. Call this via DeferCleanup in
// a Ginkgo AfterSuite.
func (m *MongoContainer) Cleanup() {
	ctx := context.Background()
	_ = m.tc.Terminate(ctx)
}
