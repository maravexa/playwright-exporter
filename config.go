// Package main implements the Playwright Prometheus exporter.
package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"regexp"
	"time"

	"gopkg.in/yaml.v3"
)

const scheduleLabel = "schedule"

// validLabelName matches valid Prometheus label names.
var validLabelName = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// reservedLabels are label names that the exporter uses internally.
var reservedLabels = map[string]bool{
	scheduleLabel: true,
	"test":        true,
	"step":        true,
}

// Config is the top-level configuration for the exporter.
type Config struct {
	ListenAddress string `yaml:"listen_address"`
	// IsolateProcessTree requests Linux per-run PID/user namespaces, without fallback.
	IsolateProcessTree bool `yaml:"isolate_process_tree"`
	// RestartOnRunFailure stops this single-schedule process on runner failure.
	RestartOnRunFailure bool             `yaml:"restart_on_run_failure"`
	Schedules           []ScheduleConfig `yaml:"schedules"`
}

// ScheduleConfig defines one scheduled Playwright test run.
type ScheduleConfig struct {
	Labels        map[string]string `yaml:"labels"`
	Name          string            `yaml:"name"`
	PlaywrightDir string            `yaml:"playwright_dir"`
	Command       string            `yaml:"command"`
	Interval      Duration          `yaml:"interval"`
	Timeout       Duration          `yaml:"timeout"`
}

// Duration wraps time.Duration to support YAML unmarshalling of Go duration strings.
type Duration struct {
	time.Duration
}

// UnmarshalYAML implements yaml.Unmarshaler for Duration.
func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	s := value.Value
	dur, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", s, err)
	}
	d.Duration = dur
	return nil
}

// ParseConfigBytes parses and validates config from raw YAML bytes.
// It is the primary entry point for testing and fuzzing.
func ParseConfigBytes(data []byte) (*Config, error) {
	var cfg Config
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("config must contain exactly one YAML document")
	}
	if err := validateConfig(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// LoadConfig reads and validates the config file at the given path.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file %q: %w", path, err)
	}
	return ParseConfigBytes(data)
}

// validateConfig checks all validation rules and applies defaults.
func validateConfig(cfg *Config) error {
	if cfg.ListenAddress == "" {
		return fmt.Errorf("listen_address must be non-empty")
	}

	if len(cfg.Schedules) == 0 {
		return fmt.Errorf("at least one schedule must be defined")
	}

	if cfg.IsolateProcessTree {
		if _, err := processTreeIsolation(); err != nil {
			return err
		}
	}
	if cfg.RestartOnRunFailure && len(cfg.Schedules) != 1 {
		return fmt.Errorf("restart_on_run_failure requires exactly one schedule")
	}

	names := make(map[string]bool, len(cfg.Schedules))
	for i := range cfg.Schedules {
		s := &cfg.Schedules[i]

		if s.Name == "" {
			return fmt.Errorf("schedule[%d]: name must be non-empty", i)
		}
		if !validLabelName.MatchString(s.Name) {
			return fmt.Errorf("schedule %q: name must be a valid Prometheus label value (alphanumeric + underscores)", s.Name)
		}
		if names[s.Name] {
			return fmt.Errorf("schedule %q: duplicate name", s.Name)
		}
		names[s.Name] = true

		if s.Interval.Duration <= 0 {
			return fmt.Errorf("schedule %q: interval must be > 0", s.Name)
		}

		if s.Timeout.Duration == 0 {
			s.Timeout.Duration = time.Duration(float64(s.Interval.Duration) * 0.8)
		} else if s.Timeout.Duration >= s.Interval.Duration {
			return fmt.Errorf("schedule %q: timeout (%s) must be less than interval (%s)", s.Name, s.Timeout.Duration, s.Interval.Duration)
		}

		if s.Timeout.Duration <= 0 {
			return fmt.Errorf("schedule %q: timeout must be > 0 after applying defaults", s.Name)
		}

		if s.PlaywrightDir == "" {
			return fmt.Errorf("schedule %q: playwright_dir must be non-empty", s.Name)
		}
		fi, err := os.Stat(s.PlaywrightDir)
		if err != nil {
			return fmt.Errorf("schedule %q: playwright_dir %q: %w", s.Name, s.PlaywrightDir, err)
		}
		if !fi.IsDir() {
			return fmt.Errorf("schedule %q: playwright_dir %q is not a directory", s.Name, s.PlaywrightDir)
		}

		if s.Command == "" {
			s.Command = "./node_modules/.bin/playwright test --reporter=json"
		}

		for k := range s.Labels {
			if !validLabelName.MatchString(k) {
				return fmt.Errorf("schedule %q: label key %q is not a valid Prometheus label name", s.Name, k)
			}
			if reservedLabels[k] {
				return fmt.Errorf("schedule %q: label key %q collides with reserved label", s.Name, k)
			}
		}

	}

	return nil
}
