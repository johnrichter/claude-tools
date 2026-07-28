package cli

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/confmap"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/posflag"
	"github.com/knadh/koanf/v2"
	"github.com/spf13/pflag"

	"github.com/johnrichter/claude-shared-tooling/go/sysops"
)

// envPrefix selects which environment variables loadConfig reads, e.g.
// CLAUDE_TOOLS_TIMEOUT for the timeout key.
const envPrefix = "CLAUDE_TOOLS_"

// defaultConfigFile is the config file loadConfig looks for when the caller
// does not pass --config. Its absence is not an error: a config file is an
// optional way to stop repeating flags, never a requirement.
const defaultConfigFile = ".claude-tools.yaml"

// Config is claude-tools' resolved settings: every field a command needs, so
// a config file or environment variable only has to state a setting once
// for every subcommand to see it.
type Config struct {
	Timeout        time.Duration `koanf:"timeout"`
	MaxStdoutBytes int           `koanf:"max_stdout_bytes"`
	MaxStderrBytes int           `koanf:"max_stderr_bytes"`
}

// defaultConfig seeds koanf's lowest-precedence layer. Every key here must
// match a Config `koanf` tag and the normalized (hyphen->underscore) form
// of the matching persistent flag, so the same setting resolves to one key
// across default/file/env/flag layers.
func defaultConfig() map[string]interface{} {
	return map[string]interface{}{
		"timeout":          30 * time.Second,
		"max_stdout_bytes": sysops.DefaultMaxCaptureBytes,
		"max_stderr_bytes": sysops.DefaultMaxCaptureBytes,
	}
}

// normalizeKey maps a flag or env name onto its canonical snake_case koanf
// key, e.g. "max-stdout-bytes" and "MAX_STDOUT_BYTES" both resolve to
// "max_stdout_bytes".
func normalizeKey(s string) string {
	return strings.ReplaceAll(strings.ToLower(s), "-", "_")
}

// loadConfig resolves Config from, in increasing precedence: built-in
// defaults, an optional YAML file, the CLAUDE_TOOLS_* environment, then
// fs's already-parsed flags. Each layer is loaded only — nothing here
// writes back to a file or the environment.
func loadConfig(fs *pflag.FlagSet) (*Config, error) {
	k := koanf.New(".")

	if err := k.Load(confmap.Provider(defaultConfig(), "."), nil); err != nil {
		return nil, fmt.Errorf("cli: load config defaults: %w", err)
	}

	if err := loadConfigFile(k, fs); err != nil {
		return nil, err
	}

	envProvider := env.Provider(envPrefix, ".", func(s string) string {
		return normalizeKey(strings.TrimPrefix(s, envPrefix))
	})
	if err := k.Load(envProvider, nil); err != nil {
		return nil, fmt.Errorf("cli: load config env: %w", err)
	}

	flagProvider := posflag.ProviderWithValue(fs, ".", k, func(key, value string) (string, interface{}) {
		return normalizeKey(key), value
	})
	if err := k.Load(flagProvider, nil); err != nil {
		return nil, fmt.Errorf("cli: load config flags: %w", err)
	}

	var cfg Config
	if err := k.Unmarshal("", &cfg); err != nil {
		return nil, fmt.Errorf("cli: unmarshal config: %w", err)
	}
	return &cfg, nil
}

// loadConfigFile loads the YAML config named by --config, or defaultConfigFile
// if present and --config was not given. An explicitly named file that does
// not exist is an error; an implicit default that does not exist is not.
func loadConfigFile(k *koanf.Koanf, fs *pflag.FlagSet) error {
	explicit := fs.Changed("config")
	path, _ := fs.GetString("config")
	if path == "" {
		path = defaultConfigFile
	}

	if _, err := os.Stat(path); err != nil {
		if explicit {
			return fmt.Errorf("cli: config file %s: %w", path, err)
		}
		return nil
	}

	if err := k.Load(file.Provider(path), yaml.Parser()); err != nil {
		return fmt.Errorf("cli: load config file %s: %w", path, err)
	}
	return nil
}
