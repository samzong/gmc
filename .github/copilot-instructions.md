# gmc - Git Message Commit CLI Tool

Always reference these instructions first and fallback to search or bash commands only when you encounter unexpected information that does not match the info here.

gmc is a Go-based CLI tool that generates high-quality commit messages using LLM (Large Language Models) like OpenAI GPT. It automates the git add and commit workflow by analyzing staged changes and generating conventional commit messages.

## Working Effectively

### Prerequisites
- Go 1.21+ is required
- Git repository context (all gmc commands must be run inside a git repository)
- OpenAI-compatible API key for LLM functionality

### Bootstrap, Build, and Test
- `go mod tidy` -- downloads dependencies (first run: ~18 seconds)
- `make fmt` -- formats code and tidies modules (~18 seconds, includes dependency download on first run)
- `make build` -- builds the binary to `./build/gmc` (~12 seconds). NEVER CANCEL. Set timeout to 30+ minutes.
- `make test` -- runs the full test suite (~18 seconds). NEVER CANCEL. Set timeout to 30+ minutes.
- `make clean` -- cleans build artifacts

### Running the Application
- Run `./build/gmc --help` to see all available commands and flags
- Basic usage: `./build/gmc` (requires staged changes and API configuration)
- Configuration: `./build/gmc config set apikey YOUR_API_KEY`
- Dry run: `./build/gmc --dry-run` (generates message without committing)
- Analysis: `./build/gmc analyze` (analyzes commit history, works without API key)

## Validation

### Test Commands That Work
- `make fmt` -- Always succeeds, formats Go code and runs `go mod tidy`
- `make test` -- Always succeeds, runs comprehensive test suite with mocking
- `make build` -- Always succeeds, creates `./build/gmc` binary
- `make clean` -- Always succeeds, removes build artifacts

### Commands With Known Issues  
- `make lint` -- **FAILS due to golangci-lint config version issue**. The `.golangci.yml` contains `version: "2"` which causes a parsing error. Do not rely on linting for validation.
- `make check` -- **FAILS due to lint dependency**. This runs fmt + lint + test, but fails on the lint step.

### Manual Testing Scenarios
After making code changes, ALWAYS test these scenarios:

1. **Basic Help and Version**:
   ```bash
   ./build/gmc --help
   ./build/gmc version
   ./build/gmc config --help
   ```

2. **Configuration Commands**:
   ```bash
   ./build/gmc config get
   ./build/gmc config set --help
   ```

3. **Error Handling Without API Key**:
   ```bash
   echo "test" > test_file.txt
   git add test_file.txt
   ./build/gmc --dry-run
   # Should show: "Error: failed to generate commit message: API key not set..."
   git reset HEAD test_file.txt && rm test_file.txt
   ```

4. **No Staged Changes Detection**:
   ```bash
   ./build/gmc
   # Should show: "Error: no changes detected in the staging area files"
   ```

5. **Analyze Command (Works Without API Key)**:
   ```bash
   ./build/gmc analyze
   # Should show commit quality analysis for local repository
   ./build/gmc analyze --help
   ```

### Quality Checks Before Committing
- Always run `make fmt` before committing
- Always run `make test` before committing  
- Always run `make build` to ensure the binary builds successfully
- **Do NOT run `make lint` or `make check`** -- they fail due to config issues

## Project Structure

### Key Directories
- `cmd/` -- Cobra CLI command definitions and main application logic
- `internal/analyzer/` -- Commit message quality analysis
- `internal/config/` -- Configuration management (viper-based)
- `internal/formatter/` -- Template rendering and prompt formatting
- `internal/git/` -- Git operations wrapper functions  
- `internal/llm/` -- OpenAI API client and LLM integration
- `build/` -- Build artifacts (created by `make build`)

### Important Files
- `main.go` -- Application entry point
- `Makefile` -- Build automation with well-defined targets
- `.golangci.yml` -- Linting configuration (has version compatibility issues)
- `go.mod` -- Go module definition requiring Go 1.24
- `.github/workflows/` -- CI/CD pipeline definitions

### Configuration
- Default config location: `$HOME/.gmc.yaml`
- Config commands: `gmc config set apikey|apibase|model|role|prompt_template|prompts_dir`
- Default prompts directory: `$HOME/.gmc/prompts`

## Common Development Tasks

### Adding New Features
1. Always modify code in the appropriate `internal/` or `cmd/` directory
2. Add corresponding tests following existing patterns
3. Run `make fmt && make test && make build` before committing
4. Test CLI functionality manually with the validation scenarios above

### Debugging Issues
1. Use `./build/gmc --verbose` for detailed git command output
2. Use `./build/gmc --dry-run` to test message generation without committing  
3. Check logs and error messages -- the application has good error handling

### Working with Tests
- Test files follow standard Go conventions (`*_test.go`)
- Tests use testify/assert for assertions
- LLM-related tests are properly mocked to avoid external API calls
- Some integration tests are skipped by default (look for `t.Skip()` calls)
- Git operation tests are designed to be safe and not perform real git operations

## Timing Expectations

- **Build**: ~12 seconds - NEVER CANCEL. Set timeout to 30+ minutes.
- **Test**: ~18 seconds - NEVER CANCEL. Set timeout to 30+ minutes.  
- **Format**: ~18 seconds (first run with dependency download)
- **Clean**: <1 second

## Troubleshooting

### Build Issues
- Ensure Go 1.21+ is installed
- Run `go mod tidy` if dependencies are missing
- Check that you're in the repository root directory

### Test Failures
- Most test failures are legitimate and should be investigated
- Some tests are skipped by design (integration tests, external API tests)
- Test failures in git operations may indicate you're not in a git repository

### Runtime Issues
- API key must be configured for core functionality
- Commands must be run within a git repository
- Staged changes are required for commit message generation

## Dependencies and External Tools

### Required at Runtime
- Git (for repository operations)
- OpenAI-compatible API access (for LLM functionality)

### Development Dependencies
- Go 1.21+ 
- Make (for build automation)
- golangci-lint (optional, current config has issues)

### Go Module Dependencies
- `github.com/spf13/cobra` -- CLI framework
- `github.com/spf13/viper` -- Configuration management  
- `github.com/sashabaranov/go-openai` -- OpenAI API client
- `github.com/stretchr/testify` -- Testing utilities
- `gopkg.in/yaml.v3` -- YAML processing

All dependencies are managed through Go modules and downloaded automatically during build.