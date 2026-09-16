package formatter

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
)

func TestBudgetedDiffPreservesFileFacts(t *testing.T) {
	for _, tt := range []struct {
		name, path, header, stats, summary string
	}{
		{"counts", "go.sum", "", "3\t4\tgo.sum", "go.sum (+3/-4)"},
		{"binary", "package-lock.json", "Binary files a/package-lock.json and b/package-lock.json differ", "",
			"package-lock.json (binary)"},
		{"added", "yarn.lock", "new file mode 100644", "1\t0\tyarn.lock", "yarn.lock (added) (+1/-0)"},
		{"chmod", "Cargo.lock", "old mode 100644\nnew mode 100755", "", "Cargo.lock (mode changed)"},
		{"rename", "new.lock", "rename from old.lock\nrename to new.lock", "1\t0\told.lock => new.lock",
			"old.lock -> new.lock (renamed)"},
		{"inferred counts", "go.sum", "", "", "go.sum (+1/-0)"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			diff := fmt.Sprintf("diff --git a/%s b/%s\n%s\n@@ -0,0 +1 @@\n+%s\n",
				tt.path, tt.path, tt.header, strings.Repeat("x", 100))
			result := truncateDiffWithStats(diff, tt.stats, 80)
			assert.Equal(t, tt.summary+"\n", result)
			assert.LessOrEqual(t, len(result), 80)
		})
	}
	source := "diff --git a/main.go b/main.go\n@@ -0,0 +1 @@\n+hello\n"
	generated := "diff --git a/go.sum b/go.sum\n@@ -0,0 +1 @@\n+" + strings.Repeat("x", 200) + "\n"
	result := truncateDiffWithStats(generated+source, "1\t0\tgo.sum\n1\t0\tmain.go", 120)
	assert.True(t, strings.HasPrefix(result, source))
	assert.Contains(t, result, "go.sum (+1/-0)")
	assert.Equal(t, source, truncateDiffWithStats(source, "", len(source)))
	result = truncateDiffWithStats(strings.Repeat("界", 20), "", 10)
	assert.True(t, utf8.ValidString(result))
	assert.True(t, strings.HasPrefix(result, "界界界"))
}
