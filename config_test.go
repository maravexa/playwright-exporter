package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadConfig_Valid(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "tests")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	content := `
listen_address: ":9115"
schedules:
  - name: my_schedule
    interval: 10m
    timeout: 8m
    playwright_dir: ` + sub + `
    command: "npx playwright test --reporter=json"
    labels:
      site: example
`
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ListenAddress != ":9115" {
		t.Errorf("listen_address: got %q, want %q", cfg.ListenAddress, ":9115")
	}
	if len(cfg.Schedules) != 1 {
		t.Fatalf("expected 1 schedule, got %d", len(cfg.Schedules))
	}
	s := cfg.Schedules[0]
	if s.Name != "my_schedule" {
		t.Errorf("name: got %q", s.Name)
	}
	if s.Interval.Duration != 10*time.Minute {
		t.Errorf("interval: got %v", s.Interval.Duration)
	}
	if s.Timeout.Duration != 8*time.Minute {
		t.Errorf("timeout: got %v", s.Timeout.Duration)
	}
}

func TestLoadConfig_DefaultTimeout(t *testing.T) {
	dir := t.TempDir()
	content := `
listen_address: ":9115"
schedules:
  - name: sched
    interval: 10m
    playwright_dir: ` + dir + `
`
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := time.Duration(float64(10*time.Minute) * 0.8)
	if cfg.Schedules[0].Timeout.Duration != want {
		t.Errorf("default timeout: got %v, want %v", cfg.Schedules[0].Timeout.Duration, want)
	}
}

func TestLoadConfig_DefaultCommand(t *testing.T) {
	dir := t.TempDir()
	content := `
listen_address: ":9115"
schedules:
  - name: sched
    interval: 5m
    playwright_dir: ` + dir + `
`
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Schedules[0].Command != "npx playwright test --reporter=json" {
		t.Errorf("default command: got %q", cfg.Schedules[0].Command)
	}
}

func TestValidateConfig_MissingListenAddress(t *testing.T) {
	cfg := &Config{
		Schedules: []ScheduleConfig{{Name: "x", Interval: Duration{time.Minute}}},
	}
	if err := validateConfig(cfg); err == nil {
		t.Error("expected error for missing listen_address")
	}
}

func TestValidateConfig_NoSchedules(t *testing.T) {
	cfg := &Config{ListenAddress: ":9115"}
	if err := validateConfig(cfg); err == nil {
		t.Error("expected error for no schedules")
	}
}

func TestValidateConfig_DuplicateName(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{
		ListenAddress: ":9115",
		Schedules: []ScheduleConfig{
			{Name: "dupe", Interval: Duration{time.Minute}, PlaywrightDir: dir},
			{Name: "dupe", Interval: Duration{time.Minute}, PlaywrightDir: dir},
		},
	}
	if err := validateConfig(cfg); err == nil {
		t.Error("expected error for duplicate name")
	}
}

func TestValidateConfig_InvalidName(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{
		ListenAddress: ":9115",
		Schedules: []ScheduleConfig{
			{Name: "bad-name!", Interval: Duration{time.Minute}, PlaywrightDir: dir},
		},
	}
	if err := validateConfig(cfg); err == nil {
		t.Error("expected error for invalid label name")
	}
}

func TestValidateConfig_TimeoutGEInterval(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{
		ListenAddress: ":9115",
		Schedules: []ScheduleConfig{
			{
				Name:          "sched",
				Interval:      Duration{5 * time.Minute},
				Timeout:       Duration{5 * time.Minute},
				PlaywrightDir: dir,
			},
		},
	}
	if err := validateConfig(cfg); err == nil {
		t.Error("expected error for timeout >= interval")
	}
}

func TestValidateConfig_NonexistentDir(t *testing.T) {
	cfg := &Config{
		ListenAddress: ":9115",
		Schedules: []ScheduleConfig{
			{
				Name:          "sched",
				Interval:      Duration{time.Minute},
				PlaywrightDir: "/nonexistent/path/that/does/not/exist",
			},
		},
	}
	if err := validateConfig(cfg); err == nil {
		t.Error("expected error for nonexistent dir")
	}
}

func TestValidateConfig_NotADirectory(t *testing.T) {
	dir := t.TempDir()
	f, err := os.CreateTemp(dir, "notadir")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	cfg := &Config{
		ListenAddress: ":9115",
		Schedules: []ScheduleConfig{
			{
				Name:          "sched",
				Interval:      Duration{time.Minute},
				PlaywrightDir: f.Name(),
			},
		},
	}
	if err := validateConfig(cfg); err == nil {
		t.Error("expected error for file path used as dir")
	}
}

func TestValidateConfig_InvalidLabelKey(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{
		ListenAddress: ":9115",
		Schedules: []ScheduleConfig{
			{
				Name:          "sched",
				Interval:      Duration{time.Minute},
				PlaywrightDir: dir,
				Labels:        map[string]string{"bad-key!": "value"},
			},
		},
	}
	if err := validateConfig(cfg); err == nil {
		t.Error("expected error for invalid label key")
	}
}

func TestValidateConfig_ReservedLabelKey(t *testing.T) {
	for _, reserved := range []string{"schedule", "test", "step"} {
		t.Run(reserved, func(t *testing.T) {
			dir := t.TempDir()
			cfg := &Config{
				ListenAddress: ":9115",
				Schedules: []ScheduleConfig{
					{
						Name:          "sched",
						Interval:      Duration{time.Minute},
						PlaywrightDir: dir,
						Labels:        map[string]string{reserved: "value"},
					},
				},
			}
			if err := validateConfig(cfg); err == nil {
				t.Errorf("expected error for reserved label %q", reserved)
			}
		})
	}
}

func TestLoadConfig_FileNotFound(t *testing.T) {
	_, err := LoadConfig("/nonexistent/config.yaml")
	if err == nil {
		t.Error("expected error for missing config file")
	}
}

func FuzzConfigParse(f *testing.F) {
	// Seed with valid config
	f.Add([]byte(`listen_address: ":9115"
schedules:
  - name: test
    interval: 5m
    timeout: 4m
    playwright_dir: /tmp
    command: "npx playwright test --reporter=json"
`))
	// Seed with minimal config
	f.Add([]byte(`listen_address: ":9115"
schedules:
  - name: a
    interval: 1m
    playwright_dir: /tmp
`))
	// Seed with empty input
	f.Add([]byte(""))
	// Seed with garbage
	f.Add([]byte("{{{invalid yaml###"))

	f.Fuzz(func(t *testing.T, data []byte) {
		// Must not panic. Errors are expected and fine.
		_, _ = ParseConfigBytes(data)
	})
}
