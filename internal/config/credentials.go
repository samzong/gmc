package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
	"github.com/zalando/go-keyring"
	"gopkg.in/yaml.v3"
)

const credentialService = "gmc"

type credentialState struct {
	key     string
	warning string
	loaded  bool
}

var credentials credentialState
var credentialWarningWriter io.Writer = os.Stderr

func SetCredentialWarningWriter(writer io.Writer) {
	credentialWarningWriter = writer
}

func warnCredential(message string) {
	if credentials.warning != "" {
		return
	}
	credentials.warning = message
	fmt.Fprintf(credentialWarningWriter, "gmc: %s\n", message)
}

func loadAPIKey() string {
	if credentials.loaded || configFilePath == "" {
		return ""
	}
	credentials.loaded = true
	account, err := credentialAccount()
	if err == nil {
		credentials.key, err = keyring.Get(credentialService, account)
	}
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return "cannot read OS credential storage. Restore access to OS credential storage, or set GMC_API_KEY"
	}
	return ""
}

func credentialAccount() (string, error) {
	path, err := filepath.Abs(configFilePath)
	if err != nil {
		return "", err
	}
	missing := ""
	for {
		resolved, err := filepath.EvalSymlinks(path)
		if err == nil {
			return filepath.Join(resolved, missing), nil
		}
		parent := filepath.Dir(path)
		if !os.IsNotExist(err) || parent == path {
			return "", err
		}
		missing = filepath.Join(filepath.Base(path), missing)
		path = parent
	}
}

func storeAPIKey(key string) error {
	if key == "" {
		return errors.New("API key cannot be empty")
	}
	account, err := credentialAccount()
	if err != nil {
		return fmt.Errorf("cannot identify the configuration file for OS credential storage: %w", err)
	}
	if err := keyring.Set(credentialService, account, key); err != nil {
		return errors.New("cannot store the API key in OS credential storage. Use GMC_API_KEY instead")
	}
	credentials.key = key
	credentials.loaded = true
	data, err := os.ReadFile(configFilePath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil || removeFileAPIKey(data) != nil {
		return errors.New("the API key is in OS credential storage, " +
			"but api_key could not be removed from the configuration file")
	}
	return nil
}

func writeConfig() error {
	settings := viper.AllSettings()
	delete(settings, "api_key")
	var next yaml.Node
	if err := next.Encode(settings); err != nil {
		return err
	}
	original, err := os.ReadFile(configFilePath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	var previous yaml.Node
	if err := yaml.Unmarshal(original, &previous); err != nil {
		return err
	}
	if len(previous.Content) > 0 && previous.Content[0].Kind == yaml.MappingNode {
		mapping := previous.Content[0].Content
		for i := 0; i < len(mapping); i += 2 {
			if strings.EqualFold(mapping[i].Value, "api_key") {
				value := mapping[i+1]
				if value.Kind == yaml.AliasNode {
					value = value.Alias
				}
				next.Content = append(next.Content, mapping[i], value)
			}
		}
	}
	data, err := yaml.Marshal(&next)
	if err != nil {
		return err
	}
	return writeConfigData(data)
}

func removeFileAPIKey(original []byte) error {
	var document yaml.Node
	if err := yaml.Unmarshal(original, &document); err != nil {
		return err
	}
	if len(document.Content) == 0 {
		return nil
	}
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return errors.New("configuration is not a YAML mapping")
	}
	mapping := document.Content[0]
	for i := 0; i < len(mapping.Content); {
		if strings.EqualFold(mapping.Content[i].Value, "api_key") {
			mapping.Content = append(mapping.Content[:i], mapping.Content[i+2:]...)
			continue
		}
		i += 2
	}
	data, err := yaml.Marshal(&document)
	if err != nil {
		return err
	}
	var values map[string]any
	if err := yaml.Unmarshal(data, &values); err != nil {
		return err
	}
	for key := range values {
		if strings.EqualFold(key, "api_key") {
			return errors.New("the file API key is not a direct YAML field")
		}
	}
	current, err := os.ReadFile(configFilePath)
	if err != nil {
		return err
	}
	if !bytes.Equal(original, current) {
		return errors.New("configuration changed while removing the file API key")
	}
	return writeConfigData(data)
}

func writeConfigData(data []byte) error {
	path := configFilePath
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".gmc-config-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
