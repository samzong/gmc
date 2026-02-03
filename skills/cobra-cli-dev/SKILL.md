---
name: cobra-cli-dev
description: Develop and review Cobra CLI commands for Go projects. Use when creating new commands, reviewing CLI code, adding flags, implementing shell completions, or debugging command behavior.
---

# Cobra CLI Development Standards

First-principles CLI architecture for production-grade open source projects.

## Core Philosophy

| Principle | Meaning | Validation Question |
|-----------|---------|---------------------|
| **Consistency** | All commands follow the same pattern | Can users guess usage intuitively? |
| **Predictability** | Same input produces same output | Can scripts call it reliably? |
| **Composability** | Output can feed into other commands | Can it be piped? |
| **Least Surprise** | Behavior matches expectations | Any "magic" behavior? |
| **Graceful Degradation** | Errors provide useful information | Does the user know how to fix it? |

## Three-Layer Architecture

```
Layer 1: Command (cmd/*.go)     - cobra.Command + Flag binding + Args validation
Layer 2: Runner Function        - Build Options from flags → Call Layer 3 → Format output
Layer 3: Business Logic (internal/*) - No CLI dependencies, testable, returns structured data
```

**Principle**: One file = one command group. Split files when subcommands > 3.

## Command Definition Template

```go
var exampleCmd = &cobra.Command{
    Use:   "example <required-arg> [optional-arg]",
    Short: "One-line description (< 50 chars)",
    Long: `Full description including functionality, use cases, and notes.`,
    Example: `  gmc example foo
  gmc example foo --verbose`,
    Args:          cobra.ExactArgs(1),
    SilenceUsage:  true,
    SilenceErrors: false,
    RunE:          runExample,
}
```

## Flag Standards

**Naming**: Use hyphens for long names `--dry-run`, single letter for short `-n`. No camelCase or underscores.

**Type Selection**:
```go
BoolVarP(&dryRun, "dry-run", "n", false, "Preview without executing")
StringVarP(&output, "output", "o", "", "Output file path")
StringSliceVarP(&tags, "tag", "t", nil, "Add tags (can be repeated)")
```

**Relationship Constraints**:
```go
cmd.MarkFlagsMutuallyExclusive("json", "yaml")   // Mutually exclusive
cmd.MarkFlagsRequiredTogether("user", "pass")    // Must be used together
cmd.MarkFlagsOneRequired("stdin", "file")        // At least one required
```

## Output Stream Separation

```go
// stdout: Primary data (pipeable)
fmt.Fprintln(cmd.OutOrStdout(), result)

// stderr: Progress, hints, errors
fmt.Fprintln(cmd.ErrOrStderr(), "Processing...")

// Forbidden: fmt.Println() / os.Stdout.Write()
```

## Error Handling

```go
// User error: Include hints
type UserError struct {
    Message string
    Hint    string
}

// System error: Wrap with context
return fmt.Errorf("failed to process %q: %w", args[0], err)

// main.go unified handling
if err := cmd.Execute(); err != nil {
    fmt.Fprintf(os.Stderr, "gmc: %v\n", err)
    os.Exit(1)
}
```

## Testing Pattern

```go
func TestExampleCommand(t *testing.T) {
    cmd := NewRootCmd()  // Factory function, avoid global state
    outBuf, errBuf := new(bytes.Buffer), new(bytes.Buffer)
    cmd.SetOut(outBuf)
    cmd.SetErr(errBuf)
    cmd.SetArgs([]string{"example", "foo"})
    err := cmd.Execute()
    // Assert...
}
```

## Shell Completion

```go
// Static completion
ValidArgs: []string{"json", "yaml", "table"}

// Dynamic completion
ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
    return getBranches(toComplete), cobra.ShellCompDirectiveNoFileComp
}

// Flag completion
cmd.RegisterFlagCompletionFunc("output", ...)
cmd.MarkFlagFilename("config", "yaml", "yml")
```

## Code Review Checklist

- [ ] Use follows POSIX syntax: `command <required> [optional]`
- [ ] Short < 50 characters
- [ ] Example field present
- [ ] Uses RunE instead of Run
- [ ] Args validator correctly set
- [ ] Flags use hyphen-case naming
- [ ] Output uses cmd.OutOrStdout()
- [ ] Errors use cmd.ErrOrStderr()
- [ ] Test cases exist
- [ ] Shell completion configured

## Common Anti-Patterns

| BAD | GOOD |
|-----|------|
| `fmt.Println(...)` | `fmt.Fprintln(cmd.OutOrStdout(), ...)` |
| `Run: func(...)` | `RunE: func(...) error` |
| `var globalFlag bool` | Closure capture or factory function |

## References

- [Cobra Official Documentation](https://cobra.dev)
- [Cobra GitHub Repository](https://github.com/spf13/cobra)
- [12 Factor CLI Apps](https://medium.com/@jdxcode/12-factor-cli-apps-dd3c227a0e46)
- [Command Line Interface Guidelines](https://clig.dev)
