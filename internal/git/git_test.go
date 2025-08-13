package git

import (
	"testing"
)

func TestParseCommitOutput(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
		wantErr  bool
	}{
		{
			name: "Valid commit output",
			input: `abc1234|John Doe|2024-01-15|feat: add new feature
def5678|Jane Smith|2024-01-14|fix: resolve bug in parser`,
			expected: 2,
			wantErr:  false,
		},
		{
			name:     "Empty input",
			input:    "",
			expected: 0,
			wantErr:  false,
		},
		{
			name:     "Single commit",
			input:    `abc1234|John Doe|2024-01-15|feat: add new feature`,
			expected: 1,
			wantErr:  false,
		},
		{
			name: "Malformed line (should be skipped)",
			input: `abc1234|John Doe|2024-01-15|feat: add new feature
invalid-line
def5678|Jane Smith|2024-01-14|fix: resolve bug`,
			expected: 2,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			commits, err := parseCommitOutput(tt.input)

			if (err != nil) != tt.wantErr {
				t.Errorf("parseCommitOutput() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if len(commits) != tt.expected {
				t.Errorf("parseCommitOutput() got %d commits, want %d", len(commits), tt.expected)
				return
			}

			// Test first commit if exists
			if len(commits) > 0 {
				commit := commits[0]
				if commit.Hash == "" || commit.Author == "" || commit.Date == "" || commit.Message == "" {
					t.Errorf("parseCommitOutput() commit has empty fields: %+v", commit)
				}
			}
		})
	}
}

func TestCommitInfoFields(t *testing.T) {
	input := `abc1234|John Doe|2024-01-15|feat: add new feature`
	commits, err := parseCommitOutput(input)

	if err != nil {
		t.Fatalf("parseCommitOutput() error = %v", err)
	}

	if len(commits) != 1 {
		t.Fatalf("parseCommitOutput() got %d commits, want 1", len(commits))
	}

	commit := commits[0]

	if commit.Hash != "abc1234" {
		t.Errorf("commit.Hash = %q, want %q", commit.Hash, "abc1234")
	}

	if commit.Author != "John Doe" {
		t.Errorf("commit.Author = %q, want %q", commit.Author, "John Doe")
	}

	if commit.Date != "2024-01-15" {
		t.Errorf("commit.Date = %q, want %q", commit.Date, "2024-01-15")
	}

	if commit.Message != "feat: add new feature" {
		t.Errorf("commit.Message = %q, want %q", commit.Message, "feat: add new feature")
	}
}
