package cmd

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfirmTagCreation(t *testing.T) {
	for _, test := range []struct {
		name  string
		auto  bool
		input string
		want  bool
	}{
		{"auto yes", true, "", true},
		{"accept", false, "y\n", true},
		{"decline", false, "n\n", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			setTestValue(t, &tagAutoYes, test.auto)
			setTestValue(t, &isStdinTerminal, func() bool { return true })
			reader, writer, err := os.Pipe()
			require.NoError(t, err)
			t.Cleanup(func() { _ = reader.Close() })
			_, err = writer.WriteString(test.input)
			require.NoError(t, err)
			require.NoError(t, writer.Close())
			setTestValue(t, &os.Stdin, reader)
			confirmed, err := confirmTagCreation("v1.2.3")
			assert.NoError(t, err)
			assert.Equal(t, test.want, confirmed)
		})
	}
}
