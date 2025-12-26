package cmd

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfirmTagCreationAutoYes(t *testing.T) {
	original := tagAutoYes
	defer func() { tagAutoYes = original }()

	tagAutoYes = true
	confirmed, err := confirmTagCreation("v1.2.3")
	assert.NoError(t, err)
	assert.True(t, confirmed)
}

func TestConfirmTagCreationUserInput(t *testing.T) {
	original := tagAutoYes
	defer func() { tagAutoYes = original }()

	originalIsStdinTerminal := isStdinTerminal
	defer func() { isStdinTerminal = originalIsStdinTerminal }()
	isStdinTerminal = func() bool { return true }

	tagAutoYes = false

	// Mock stdin with affirmative answer
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	defer reader.Close()

	_, _ = writer.WriteString("y\n")
	writer.Close()

	originalStdin := os.Stdin
	defer func() { os.Stdin = originalStdin }()
	os.Stdin = reader

	confirmed, err := confirmTagCreation("v1.2.3")
	assert.NoError(t, err)
	assert.True(t, confirmed)
}

func TestConfirmTagCreationDecline(t *testing.T) {
	original := tagAutoYes
	defer func() { tagAutoYes = original }()

	originalIsStdinTerminal := isStdinTerminal
	defer func() { isStdinTerminal = originalIsStdinTerminal }()
	isStdinTerminal = func() bool { return true }

	tagAutoYes = false

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	defer reader.Close()

	_, _ = writer.WriteString("n\n")
	writer.Close()

	originalStdin := os.Stdin
	defer func() { os.Stdin = originalStdin }()
	os.Stdin = reader

	confirmed, err := confirmTagCreation("v1.2.3")
	assert.NoError(t, err)
	assert.False(t, confirmed)
}
