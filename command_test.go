package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestResolveLocalPlaywright(t *testing.T) {
	project := filepath.Join(t.TempDir(), "project with spaces")
	bin := filepath.Join(project, "node_modules", ".bin", "playwright")
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(project, "cli.js")
	if err := os.Symlink("/bin/sh", target); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, bin); err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{"playwright test --reporter=json", "npx playwright test --reporter=json", "/usr/bin/npx playwright test --reporter=json", "./node_modules/.bin/playwright test --reporter=json"} {
		t.Run(command, func(t *testing.T) {
			name, args, err := resolveScheduleCommand(&ScheduleConfig{Name: "local", PlaywrightDir: project, Command: command})
			if err != nil {
				t.Fatal(err)
			}
			if name != bin || !reflect.DeepEqual(args, []string{"test", "--reporter=json"}) {
				t.Fatalf("got %q %v", name, args)
			}
		})
	}
}

func TestMissingPlaywrightDoesNotSpawn(t *testing.T) {
	for _, command := range []string{"npx playwright test", "playwright test", "./node_modules/.bin/playwright test"} {
		t.Run(command, func(t *testing.T) {
			ex := &Executor{buildCmd: func(context.Context, string, ...string) *exec.Cmd {
				t.Fatal("must not spawn a downloader or PATH executable")
				return nil
			}}
			_, err := ex.Run(context.Background(), &ScheduleConfig{Name: "missing", PlaywrightDir: t.TempDir(), Command: command})
			if err == nil || !strings.Contains(err.Error(), "reviewed locked dependencies") {
				t.Fatalf("got %v", err)
			}
		})
	}
}

func TestRejectDownloadOptionsAndGlobalPlaywright(t *testing.T) {
	for _, command := range []string{"npx --yes playwright test", "npx playwright@latest test", "npx --no-install playwright test", "npx", "/usr/bin/playwright test"} {
		_, _, err := resolveScheduleCommand(&ScheduleConfig{Name: "reject", PlaywrightDir: t.TempDir(), Command: command})
		if err == nil {
			t.Fatalf("accepted %q", command)
		}
	}
}

func TestInvalidLocalExecutable(t *testing.T) {
	for _, kind := range []string{"directory", "not-executable", "dangling-symlink"} {
		t.Run(kind, func(t *testing.T) {
			project := t.TempDir()
			bin := filepath.Join(project, "node_modules", ".bin", "playwright")
			if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
				t.Fatal(err)
			}
			var err error
			switch kind {
			case "directory":
				err = os.Mkdir(bin, 0o755)
			case "not-executable":
				err = os.WriteFile(bin, []byte("text"), 0o600)
			case "dangling-symlink":
				err = os.Symlink("missing", bin)
			}
			if err != nil {
				t.Fatal(err)
			}
			_, _, err = resolveScheduleCommand(&ScheduleConfig{Name: "broken", PlaywrightDir: project, Command: "playwright test"})
			if err == nil || !strings.Contains(err.Error(), "no package download attempted") {
				t.Fatalf("got %v", err)
			}
		})
	}
}

func TestCustomCommandPreserved(t *testing.T) {
	name, args, err := resolveScheduleCommand(&ScheduleConfig{Command: "./custom-runner --reporter=json"})
	if err != nil || name != "./custom-runner" || !reflect.DeepEqual(args, []string{"--reporter=json"}) {
		t.Fatalf("got %q %v %v", name, args, err)
	}
}

// Opt-in integration test against an already provisioned isolated project.
// This test never installs dependencies or browsers.
func TestExecutorLocalPlaywrightIntegration(t *testing.T) {
	project := os.Getenv("PLAYWRIGHT_EXPORTER_TEST_PROJECT")
	if project == "" {
		t.Skip("set PLAYWRIGHT_EXPORTER_TEST_PROJECT to an isolated provisioned fixture")
	}
	for _, command := range []string{"./node_modules/.bin/playwright test --reporter=json", "npx playwright test --reporter=json"} {
		t.Run(command, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			result, err := NewExecutor().Run(ctx, &ScheduleConfig{Name: "integration", PlaywrightDir: project, Command: command})
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Tests) != 2 {
				t.Fatalf("expected CJS and ESM fixture results, got %d", len(result.Tests))
			}
			for _, test := range result.Tests {
				if !test.Passed {
					t.Errorf("fixture failed: %s", test.Name)
				}
			}
		})
	}
}
