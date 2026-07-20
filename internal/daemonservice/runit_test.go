package daemonservice

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kardianos/service"
)

// @description    Creates a simulated Termux prefix with required runit executables.
//
// @param           t  "test handle used to create temporary files"
//
// @return          string  "temporary Termux prefix path"
//
// @return          string  "temporary daemon executable path"
func prepareRunitPrefix(t *testing.T) (string, string) {
	t.Helper()
	prefix := t.TempDir()
	for _, dir := range []string{
		filepath.Join(prefix, "bin"),
		filepath.Join(prefix, "var", "service"),
		filepath.Join(prefix, "share", "termux-services"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) returned error %v", dir, err)
		}
	}
	for _, path := range []string{
		filepath.Join(prefix, "bin", "sv"),
		filepath.Join(prefix, "share", "termux-services", "svlogger"),
	} {
		if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatalf("WriteFile(%q) returned error %v", path, err)
		}
	}
	daemonPath := filepath.Join(prefix, "bin", "git-auto-sync-daemon")
	if err := os.WriteFile(daemonPath, []byte("daemon"), 0o755); err != nil {
		t.Fatalf("WriteFile(%q) returned error %v", daemonPath, err)
	}
	return prefix, daemonPath
}

// @description    Verifies runit installation creates a marked executable service definition.
//
// @param           t  "test handle used for filesystem assertions"
func TestRunitBackendInstallCreatesManagedService(t *testing.T) {
	prefix, daemonPath := prepareRunitPrefix(t)
	backend, err := newRunitBackend(prefix, daemonPath, func(string, ...string) ([]byte, error) {
		return nil, nil
	})
	if err != nil {
		t.Fatalf("newRunitBackend returned error %v, want nil", err)
	}

	if err := backend.Install(); err != nil {
		t.Fatalf("Install returned error %v, want nil", err)
	}

	marker, err := os.ReadFile(filepath.Join(backend.serviceDir, managedMarkerName))
	if err != nil {
		t.Fatalf("ReadFile(marker) returned error %v", err)
	}
	if strings.TrimSpace(string(marker)) != managedMarkerContent {
		t.Fatalf("marker content = %q, want %q", marker, managedMarkerContent)
	}
	runInfo, err := os.Stat(filepath.Join(backend.serviceDir, "run"))
	if err != nil {
		t.Fatalf("Stat(run) returned error %v", err)
	}
	if runInfo.Mode().Perm() != 0o755 {
		t.Fatalf("run mode = %o, want 755", runInfo.Mode().Perm())
	}
	runScript, err := os.ReadFile(filepath.Join(backend.serviceDir, "run"))
	if err != nil {
		t.Fatalf("ReadFile(run) returned error %v", err)
	}
	if !strings.Contains(string(runScript), daemonPath) {
		t.Fatalf("run script = %q, want daemon path %q", runScript, daemonPath)
	}
	link, err := os.Readlink(filepath.Join(backend.serviceDir, "log", "run"))
	if err != nil {
		t.Fatalf("Readlink(log/run) returned error %v", err)
	}
	if link != filepath.Join(prefix, "share", "termux-services", "svlogger") {
		t.Fatalf("logger link = %q, want Termux svlogger", link)
	}
}

// @description    Verifies runit status output maps to shared service status values.
//
// @param           t  "test handle used for status assertions"
func TestRunitBackendStatusParsesRunAndDown(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   service.Status
	}{
		{name: "running", output: "run: git-auto-sync-daemon: (pid 42) 3s\n", want: service.StatusRunning},
		{name: "stopped", output: "down: git-auto-sync-daemon: 8s, normally up\n", want: service.StatusStopped},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prefix, daemonPath := prepareRunitPrefix(t)
			backend, err := newRunitBackend(prefix, daemonPath, func(string, ...string) ([]byte, error) {
				return []byte(tt.output), nil
			})
			if err != nil {
				t.Fatalf("newRunitBackend returned error %v, want nil", err)
			}
			if err := backend.Install(); err != nil {
				t.Fatalf("Install returned error %v, want nil", err)
			}

			got, err := backend.Status()
			if err != nil {
				t.Fatalf("Status returned error %v, want nil", err)
			}
			if got != tt.want {
				t.Fatalf("Status = %v, want %v", got, tt.want)
			}
		})
	}
}

// @description    Verifies an absent runit service maps to ErrNotInstalled.
//
// @param           t  "test handle used for error assertions"
func TestRunitBackendStatusReturnsNotInstalled(t *testing.T) {
	prefix, daemonPath := prepareRunitPrefix(t)
	backend, err := newRunitBackend(prefix, daemonPath, func(string, ...string) ([]byte, error) {
		return nil, nil
	})
	if err != nil {
		t.Fatalf("newRunitBackend returned error %v, want nil", err)
	}

	_, err = backend.Status()
	if !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("Status error = %v, want ErrNotInstalled", err)
	}
}

// @description    Verifies runit start and stop use absolute sv and service paths.
//
// @param           t  "test handle used for command assertions"
func TestRunitBackendStartAndStopInvokeSV(t *testing.T) {
	prefix, daemonPath := prepareRunitPrefix(t)
	var calls [][]string
	backend, err := newRunitBackend(prefix, daemonPath, func(path string, args ...string) ([]byte, error) {
		call := append([]string{path}, args...)
		calls = append(calls, call)
		return nil, nil
	})
	if err != nil {
		t.Fatalf("newRunitBackend returned error %v, want nil", err)
	}

	if err := backend.Start(); err != nil {
		t.Fatalf("Start returned error %v, want nil", err)
	}
	if err := backend.Stop(); err != nil {
		t.Fatalf("Stop returned error %v, want nil", err)
	}

	want := [][]string{
		{filepath.Join(prefix, "bin", "sv"), "up", backend.serviceDir},
		{filepath.Join(prefix, "bin", "sv"), "down", backend.serviceDir},
	}
	if len(calls) != len(want) {
		t.Fatalf("sv calls = %q, want %q", calls, want)
	}
	for i := range want {
		if strings.Join(calls[i], "\x00") != strings.Join(want[i], "\x00") {
			t.Fatalf("sv call %d = %q, want %q", i, calls[i], want[i])
		}
	}
}

// @description    Verifies runit installation refuses to overwrite a user-managed service.
//
// @param           t  "test handle used for ownership assertions"
func TestRunitBackendInstallRejectsUnmanagedService(t *testing.T) {
	prefix, daemonPath := prepareRunitPrefix(t)
	backend, err := newRunitBackend(prefix, daemonPath, func(string, ...string) ([]byte, error) {
		return nil, nil
	})
	if err != nil {
		t.Fatalf("newRunitBackend returned error %v, want nil", err)
	}
	if err := os.MkdirAll(backend.serviceDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(serviceDir) returned error %v", err)
	}
	customRun := filepath.Join(backend.serviceDir, "run")
	if err := os.WriteFile(customRun, []byte("custom\n"), 0o755); err != nil {
		t.Fatalf("WriteFile(custom run) returned error %v", err)
	}

	err = backend.Install()
	if err == nil || !strings.Contains(err.Error(), "refusing to overwrite unmanaged runit service") {
		t.Fatalf("Install error = %v, want unmanaged-service refusal", err)
	}
	content, readErr := os.ReadFile(customRun)
	if readErr != nil {
		t.Fatalf("ReadFile(custom run) returned error %v", readErr)
	}
	if string(content) != "custom\n" {
		t.Fatalf("custom run content = %q, want unchanged", content)
	}
}

// @description    Verifies uninstall removes only the managed service definition and preserves logs.
//
// @param           t  "test handle used for filesystem assertions"
func TestRunitBackendUninstallPreservesServiceLogs(t *testing.T) {
	prefix, daemonPath := prepareRunitPrefix(t)
	backend, err := newRunitBackend(prefix, daemonPath, func(string, ...string) ([]byte, error) {
		return nil, nil
	})
	if err != nil {
		t.Fatalf("newRunitBackend returned error %v, want nil", err)
	}
	if err := backend.Install(); err != nil {
		t.Fatalf("Install returned error %v, want nil", err)
	}
	logFile := filepath.Join(prefix, "var", "log", "sv", daemonServiceName, "current")
	if err := os.MkdirAll(filepath.Dir(logFile), 0o755); err != nil {
		t.Fatalf("MkdirAll(log dir) returned error %v", err)
	}
	if err := os.WriteFile(logFile, []byte("log\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(log) returned error %v", err)
	}

	if err := backend.Uninstall(); err != nil {
		t.Fatalf("Uninstall returned error %v, want nil", err)
	}
	if _, err := os.Stat(backend.serviceDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("service directory stat error = %v, want not exist", err)
	}
	if _, err := os.Stat(logFile); err != nil {
		t.Fatalf("service log was not preserved: %v", err)
	}
}

// @description    Verifies an invalid service root produces actionable Termux guidance.
//
// @param           t  "test handle used for dependency error assertions"
func TestNewRunitBackendRejectsNonDirectoryServiceRoot(t *testing.T) {
	prefix, daemonPath := prepareRunitPrefix(t)
	serviceRoot := filepath.Join(prefix, "var", "service")
	if err := os.RemoveAll(serviceRoot); err != nil {
		t.Fatalf("RemoveAll(service root) returned error %v", err)
	}
	if err := os.WriteFile(serviceRoot, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("WriteFile(service root) returned error %v", err)
	}

	_, err := newRunitBackend(prefix, daemonPath, func(string, ...string) ([]byte, error) {
		return nil, nil
	})
	if err == nil || !strings.Contains(err.Error(), "pkg install termux-services") {
		t.Fatalf("newRunitBackend error = %v, want Termux installation guidance", err)
	}
	if strings.Contains(err.Error(), "%!w") {
		t.Fatalf("newRunitBackend error contains malformed wrapping: %v", err)
	}
}

// @description    Verifies sv failures explain how to start the Termux service supervisor.
//
// @param           t  "test handle used for runtime guidance assertions"
func TestRunitBackendStatusProvidesSupervisorGuidance(t *testing.T) {
	prefix, daemonPath := prepareRunitPrefix(t)
	backend, err := newRunitBackend(prefix, daemonPath, func(string, ...string) ([]byte, error) {
		return nil, errors.New("supervise unavailable")
	})
	if err != nil {
		t.Fatalf("newRunitBackend returned error %v, want nil", err)
	}
	if err := backend.Install(); err != nil {
		t.Fatalf("Install returned error %v, want nil", err)
	}

	_, err = backend.Status()
	if err == nil || !strings.Contains(err.Error(), "start-services.sh") {
		t.Fatalf("Status error = %v, want service supervisor startup guidance", err)
	}
}
