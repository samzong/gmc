package llm

import (
	"os"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"
)

func TestMain(m *testing.M) {
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(strings.ToUpper(name), "GMC_") {
			if err := os.Unsetenv(name); err != nil {
				panic(err)
			}
		}
	}
	keyring.MockInit()
	os.Exit(m.Run())
}
