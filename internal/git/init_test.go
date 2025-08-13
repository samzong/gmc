package git

import (
	"os"
	"testing"
)

// TestMain runs before all tests in this package
func TestMain(m *testing.M) {
	// Set test environment flag to enable safety checks
	os.Setenv("GO_TEST_ENV", "1")

	// Run tests
	code := m.Run()

	// Clean up
	os.Unsetenv("GO_TEST_ENV")

	// Exit with test result code
	os.Exit(code)
}
