package llm

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestTestConnection_MissingAPIKey(t *testing.T) {
	viper.Reset()
	viper.Set("api_key", "")

	err := TestConnection("gpt-4.1-mini")
	assert.ErrorIs(t, err, errMissingAPIKey)
}
