package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/samzong/gmc/internal/config"
	"github.com/samzong/gmc/internal/llm"
	"github.com/spf13/cobra"
)

var (
	initCmd = &cobra.Command{
		Use:   "init",
		Short: "Initialize gmc configuration",
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg, err := config.GetConfig()
			if err != nil {
				return err
			}
			if err := runInitWizard(os.Stdin, outWriter(), cfg); err != nil {
				return err
			}
			fmt.Fprintln(outWriter(), "Initialization complete.")
			return nil
		},
	}

	saveConfigValues = func(apiKey, model, apiBase string) error {
		config.SetConfigValue("api_key", apiKey)
		config.SetConfigValue("model", model)
		config.SetConfigValue("api_base", apiBase)
		return config.SaveConfig()
	}

	testLLMConnection = func(model string) error {
		client := llm.NewClient(llm.Options{Timeout: time.Duration(timeoutSeconds) * time.Second})
		return client.TestConnection(model)
	}
)

func runInitWizard(in io.Reader, out io.Writer, current *config.Config) error {
	cfg, err := initWizardConfig(current)
	if err != nil {
		return err
	}
	readLine := newTrimmedLineReader(in)
	fmt.Fprintln(out, "gmc init - configure your LLM settings")

	apiKey, err := promptAPIKey(out, cfg, readLine)
	if err != nil {
		return err
	}
	model, err := promptModel(out, cfg, readLine)
	if err != nil {
		return err
	}
	apiBase, err := promptAPIBase(out, cfg, readLine)
	if err != nil {
		return err
	}

	if err := saveConfigValues(apiKey, model, apiBase); err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}

	return maybeTestConnection(out, model, readLine)
}

func initWizardConfig(current *config.Config) (*config.Config, error) {
	if current != nil {
		return current, nil
	}
	return config.GetConfig()
}

func newTrimmedLineReader(in io.Reader) func() (string, error) {
	reader := bufio.NewReader(in)
	return func() (string, error) {
		line, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return "", err
		}
		if errors.Is(err, io.EOF) && line == "" {
			return "", io.EOF
		}
		return strings.TrimSpace(line), nil
	}
}

func promptAPIKey(out io.Writer, cfg *config.Config, readLine func() (string, error)) (string, error) {
	for {
		if cfg.APIKey != "" {
			fmt.Fprint(out, "OpenAI API Key (leave blank to keep current): ")
		} else {
			fmt.Fprint(out, "OpenAI API Key (required): ")
		}

		line, err := readLine()
		if err != nil {
			return "", err
		}
		if line == "" {
			if cfg.APIKey != "" {
				return cfg.APIKey, nil
			}
			fmt.Fprintln(out, "API key is required.")
			continue
		}
		return line, nil
	}
}

func promptModel(out io.Writer, cfg *config.Config, readLine func() (string, error)) (string, error) {
	modelDefault := cfg.Model
	if modelDefault == "" {
		modelDefault = config.DefaultModel
	}
	fmt.Fprintf(out, "Model (default: %s): ", modelDefault)

	line, err := readLine()
	if err != nil {
		return "", err
	}
	if line == "" {
		return modelDefault, nil
	}
	return line, nil
}

func promptAPIBase(out io.Writer, cfg *config.Config, readLine func() (string, error)) (string, error) {
	apiBaseLabel := cfg.APIBase
	if apiBaseLabel == "" {
		apiBaseLabel = "<empty>"
	}
	fmt.Fprintf(out, "API Base URL (default: %s): ", apiBaseLabel)

	line, err := readLine()
	if err != nil {
		return "", err
	}
	if line == "" {
		return cfg.APIBase, nil
	}
	return line, nil
}

func maybeTestConnection(out io.Writer, model string, readLine func() (string, error)) error {
	for {
		fmt.Fprint(out, "Test API connection now? [Y/n]: ")
		answer, err := readLine()
		if err != nil {
			return err
		}
		switch strings.ToLower(answer) {
		case "", "y", "yes":
			fmt.Fprintln(out, "Testing API connection...")
			if err := testLLMConnection(model); err != nil {
				fmt.Fprintf(out, "Connection test failed: %v\n", err)
				fmt.Fprintln(out, "You can re-run `gmc init` or update config with `gmc config set`.")
			} else {
				fmt.Fprintln(out, "Connection test succeeded.")
			}
			return nil
		case "n", "no":
			return nil
		default:
			fmt.Fprintln(out, "Please enter y or n.")
		}
	}
}

func ensureLLMConfigured(
	cfg *config.Config, in io.Reader, out io.Writer,
	initRunner func(io.Reader, io.Writer, *config.Config) error,
) (bool, error) {
	current := cfg
	if current == nil {
		var err error
		current, err = config.GetConfig()
		if err != nil {
			return false, err
		}
	}
	if strings.TrimSpace(current.APIKey) != "" {
		return true, nil
	}

	fmt.Fprintln(out, "API key is not configured.")
	fmt.Fprintln(out, "An API key is required to generate commit messages.")

	reader := bufio.NewReader(in)
	for {
		fmt.Fprint(out, "Run `gmc init` now? [Y/n]: ")
		line, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return false, err
		}
		if errors.Is(err, io.EOF) && strings.TrimSpace(line) == "" {
			fmt.Fprintln(out, "Initialization skipped. Run `gmc init` anytime to configure.")
			return false, nil
		}

		answer := strings.ToLower(strings.TrimSpace(line))
		switch answer {
		case "", "y", "yes":
			if err := initRunner(reader, out, current); err != nil {
				return false, err
			}
			return true, nil
		case "n", "no":
			fmt.Fprintln(out, "Initialization skipped. Run `gmc init` anytime to configure.")
			return false, nil
		default:
			fmt.Fprintln(out, "Please enter y or n.")
		}
	}
}
