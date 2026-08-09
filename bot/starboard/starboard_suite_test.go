package starboard_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/VTGare/Eugen/testutil"
)

// discardWriter is a sink for log output in tests that don't use a session.
type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }

func TestStarboard(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Starboard Suite")
}

var mongoContainer *testutil.MongoContainer

var _ = BeforeSuite(func() {
	mongoContainer = testutil.StartMongoDB(GinkgoT())
	DeferCleanup(func() {
		mongoContainer.Cleanup()
	})
})