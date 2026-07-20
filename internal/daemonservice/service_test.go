package daemonservice

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/kardianos/service"
)

// fakeService embeds service.Service so tests only override the methods they exercise. Methods that
// are not overridden panic if called, exposing unintended service interactions during tests.
type fakeService struct {
	service.Service
	status       service.Status
	statusErr    error
	installErr   error
	uninstallErr error
	startErr     error
	stopErr      error
}

func (f *fakeService) Status() (service.Status, error) { return f.status, f.statusErr }
func (f *fakeService) Install() error                  { return f.installErr }
func (f *fakeService) Uninstall() error                { return f.uninstallErr }
func (f *fakeService) Start() error                    { return f.startErr }
func (f *fakeService) Stop() error                     { return f.stopErr }

// captureSlog runs fn with slog's default logger replaced by a text handler writing to a buffer,
// restoring the original logger afterwards.
func captureSlog(t *testing.T, fn func()) string {
	t.Helper()
	orig := slog.Default()
	var buf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	defer slog.SetDefault(orig)
	fn()
	return buf.String()
}

// @description    Verifies windowsUserEnvVars captures the user profile env vars.
//
// @param           t  "test handle used for assertions"
func TestWindowsUserEnvVarsCapturesProfileDirs(t *testing.T) {
	t.Setenv("APPDATA", `C:\Users\tester\AppData\Roaming`)
	t.Setenv("LOCALAPPDATA", `C:\Users\tester\AppData\Local`)
	t.Setenv("USERPROFILE", `C:\Users\tester`)
	t.Setenv("Path", `C:\Windows\system32;D:\Git\bin`)

	envVars := windowsUserEnvVars()

	if got := envVars["APPDATA"]; got != `C:\Users\tester\AppData\Roaming` {
		t.Fatalf("APPDATA = %q, want the CLI process value", got)
	}
	if got := envVars["LOCALAPPDATA"]; got != `C:\Users\tester\AppData\Local` {
		t.Fatalf("LOCALAPPDATA = %q, want the CLI process value", got)
	}
	if got := envVars["USERPROFILE"]; got != `C:\Users\tester` {
		t.Fatalf("USERPROFILE = %q, want the CLI process value", got)
	}
	if got := envVars["Path"]; got != `C:\Windows\system32;D:\Git\bin` {
		t.Fatalf("Path = %q, want the CLI process value", got)
	}
}

// @description    Verifies windowsUserEnvVars omits unset profile env vars.
//
// @param           t  "test handle used for assertions"
func TestWindowsUserEnvVarsOmitsEmpties(t *testing.T) {
	t.Setenv("APPDATA", "")
	t.Setenv("LOCALAPPDATA", "")
	t.Setenv("USERPROFILE", "")
	t.Setenv("Path", "")

	envVars := windowsUserEnvVars()

	if len(envVars) != 0 {
		t.Fatalf("envVars = %v, want an empty map when no profile env vars are set", envVars)
	}
}

// @description    Verifies Status returns ErrNotInstalled for a not-installed service.
//
// @param           t  "test handle used for assertions"
func TestStatusNotInstalledReturnsSentinel(t *testing.T) {
	srv := Service{backend: &fakeService{statusErr: errors.New("the service is not installed")}}

	status, err := srv.Status()

	if status != service.StatusUnknown {
		t.Fatalf("Status status = %v, want StatusUnknown", status)
	}
	if !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("Status err = %v, want ErrNotInstalled", err)
	}
}

// @description    Verifies Status returns the known status and nil error for running and stopped services.
//
// @param           t  "test handle used for assertions"
func TestStatusKnownStatesReturnStatus(t *testing.T) {
	tests := []struct {
		name   string
		status service.Status
	}{
		{name: "running", status: service.StatusRunning},
		{name: "stopped", status: service.StatusStopped},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := Service{backend: &fakeService{status: tt.status}}

			status, err := srv.Status()

			if status != tt.status {
				t.Fatalf("Status status = %v, want %v", status, tt.status)
			}
			if err != nil {
				t.Fatalf("Status err = %v, want nil", err)
			}
		})
	}
}

// @description    Verifies Status wraps non-not-installed errors rather than reporting ErrNotInstalled.
//
// @param           t  "test handle used for assertions"
func TestStatusPropagatesOtherErrors(t *testing.T) {
	srv := Service{backend: &fakeService{statusErr: errors.New("permission denied")}}

	status, err := srv.Status()

	if status != service.StatusUnknown {
		t.Fatalf("Status status = %v, want StatusUnknown", status)
	}
	if err == nil {
		t.Fatal("Status err = nil, want an error")
	}
	if errors.Is(err, ErrNotInstalled) {
		t.Fatalf("Status err = %v, should not be ErrNotInstalled", err)
	}
}

// @description    Verifies EnsureRunning logs install and start steps and returns nil for a missing service.
//
// @param           t  "test handle used for assertions"
func TestEnsureRunningLogsInstallAndStart(t *testing.T) {
	srv := Service{backend: &fakeService{statusErr: errors.New("the service is not installed")}}

	logs := captureSlog(t, func() {
		if err := srv.EnsureRunning(); err != nil {
			t.Fatalf("EnsureRunning returned error %v, want nil", err)
		}
	})

	for _, want := range []string{"Installing git-auto-sync as a daemon", "Starting git-auto-sync-daemon"} {
		if !strings.Contains(logs, want) {
			t.Fatalf("EnsureRunning logs = %q, want to contain %q", logs, want)
		}
	}
}

// @description    Verifies EnsureRunning logs the recovery attempt when installation reports an existing init entry.
//
// @param           t  "test handle used for assertions"
func TestEnsureRunningLogsReinstallRecovery(t *testing.T) {
	srv := Service{backend: &fakeService{
		statusErr:  errors.New("the service is not installed"),
		installErr: errors.New("Init already exists"),
	}}

	logs := captureSlog(t, func() {
		if err := srv.EnsureRunning(); err != nil {
			t.Fatalf("EnsureRunning returned error %v, want nil", err)
		}
	})

	if !strings.Contains(logs, "reinstalling git-auto-sync-daemon") {
		t.Fatalf("EnsureRunning logs = %q, want to contain the reinstall recovery message", logs)
	}
	if !strings.Contains(logs, "Starting git-auto-sync-daemon") {
		t.Fatalf("EnsureRunning logs = %q, want to contain the start message", logs)
	}
}

// @description    Verifies Stop logs the stop step and returns nil without uninstalling.
//
// @param           t  "test handle used for assertions"
func TestStopLogsStopOnly(t *testing.T) {
	srv := Service{backend: &fakeService{}}

	logs := captureSlog(t, func() {
		if err := srv.Stop(); err != nil {
			t.Fatalf("Stop returned error %v, want nil", err)
		}
	})

	if !strings.Contains(logs, "Stopping git-auto-sync-daemon") {
		t.Fatalf("Stop logs = %q, want to contain the stop message", logs)
	}
	if strings.Contains(logs, "Uninstalling") {
		t.Fatalf("Stop logs = %q, should not uninstall", logs)
	}
}

// @description    Verifies Stop wraps a stop error and returns it.
//
// @param           t  "test handle used for assertions"
func TestStopPropagatesError(t *testing.T) {
	srv := Service{backend: &fakeService{stopErr: errors.New("stop failed")}}

	err := srv.Stop()

	if err == nil {
		t.Fatal("Stop err = nil, want an error")
	}
}

// @description    Verifies Disable logs stop and uninstall steps for a running service and returns nil.
//
// @param           t  "test handle used for assertions"
func TestDisableLogsStopAndUninstall(t *testing.T) {
	srv := Service{backend: &fakeService{status: service.StatusRunning}}

	logs := captureSlog(t, func() {
		if err := srv.Disable(); err != nil {
			t.Fatalf("Disable returned error %v, want nil", err)
		}
	})

	for _, want := range []string{"Stopping git-auto-sync-daemon", "Uninstalling git-auto-sync as a daemon"} {
		if !strings.Contains(logs, want) {
			t.Fatalf("Disable logs = %q, want to contain %q", logs, want)
		}
	}
}

// @description    Verifies Disable uninstalls a stopped service without calling Stop.
//
// @param           t  "test handle used for assertions"
func TestDisableStoppedSkipsStop(t *testing.T) {
	srv := Service{backend: &fakeService{status: service.StatusStopped}}

	logs := captureSlog(t, func() {
		if err := srv.Disable(); err != nil {
			t.Fatalf("Disable returned error %v, want nil", err)
		}
	})

	if strings.Contains(logs, "Stopping") {
		t.Fatalf("Disable logs = %q, should not stop an already-stopped service", logs)
	}
	if !strings.Contains(logs, "Uninstalling git-auto-sync as a daemon") {
		t.Fatalf("Disable logs = %q, want to contain the uninstall message", logs)
	}
}

// @description    Verifies Disable returns nil without stopping or uninstalling when the service is not installed.
//
// @param           t  "test handle used for assertions"
func TestDisableNotInstalledReturnsNil(t *testing.T) {
	srv := Service{backend: &fakeService{statusErr: errors.New("the service is not installed")}}

	logs := captureSlog(t, func() {
		if err := srv.Disable(); err != nil {
			t.Fatalf("Disable returned error %v, want nil", err)
		}
	})

	if strings.Contains(logs, "Stopping") || strings.Contains(logs, "Uninstalling") {
		t.Fatalf("Disable logs = %q, should not touch a not-installed service", logs)
	}
}

// @description    Verifies Disable wraps a stop error from a running service and returns it.
//
// @param           t  "test handle used for assertions"
func TestDisablePropagatesStopError(t *testing.T) {
	srv := Service{backend: &fakeService{status: service.StatusRunning, stopErr: errors.New("stop failed")}}

	if err := srv.Disable(); err == nil {
		t.Fatal("Disable err = nil, want an error")
	}
}

// @description    Verifies Disable wraps an uninstall error and returns it.
//
// @param           t  "test handle used for assertions"
func TestDisablePropagatesUninstallError(t *testing.T) {
	srv := Service{backend: &fakeService{status: service.StatusStopped, uninstallErr: errors.New("uninstall failed")}}

	if err := srv.Disable(); err == nil {
		t.Fatal("Disable err = nil, want an error")
	}
}

// @description    Verifies Restart stops then starts a running service.
//
// @param           t  "test handle used for assertions"
func TestRestartRunningStopsAndStarts(t *testing.T) {
	srv := Service{backend: &fakeService{status: service.StatusRunning}}

	logs := captureSlog(t, func() {
		if err := srv.Restart(); err != nil {
			t.Fatalf("Restart returned error %v, want nil", err)
		}
	})

	for _, want := range []string{"Stopping git-auto-sync-daemon", "Restarting git-auto-sync-daemon"} {
		if !strings.Contains(logs, want) {
			t.Fatalf("Restart logs = %q, want to contain %q", logs, want)
		}
	}
}

// @description    Verifies Restart starts a stopped service without stopping.
//
// @param           t  "test handle used for assertions"
func TestRestartStoppedStartsOnly(t *testing.T) {
	srv := Service{backend: &fakeService{status: service.StatusStopped}}

	logs := captureSlog(t, func() {
		if err := srv.Restart(); err != nil {
			t.Fatalf("Restart returned error %v, want nil", err)
		}
	})

	if !strings.Contains(logs, "Restarting git-auto-sync-daemon") {
		t.Fatalf("Restart logs = %q, want to contain restart message", logs)
	}
	if strings.Contains(logs, "Stopping") {
		t.Fatalf("Restart logs = %q, should not stop an already-stopped service", logs)
	}
}

// @description    Verifies Restart returns ErrNotInstalled for a not-installed service.
//
// @param           t  "test handle used for assertions"
func TestRestartNotInstalledReturnsSentinel(t *testing.T) {
	srv := Service{backend: &fakeService{statusErr: errors.New("the service is not installed")}}

	err := srv.Restart()

	if !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("Restart err = %v, want ErrNotInstalled", err)
	}
}

// @description    Verifies Restart propagates a stop error.
//
// @param           t  "test handle used for assertions"
func TestRestartPropagatesStopError(t *testing.T) {
	srv := Service{backend: &fakeService{status: service.StatusRunning, stopErr: errors.New("stop failed")}}

	if err := srv.Restart(); err == nil {
		t.Fatal("Restart err = nil, want an error")
	}
}

// @description    Verifies Restart propagates a start error.
//
// @param           t  "test handle used for assertions"
func TestRestartPropagatesStartError(t *testing.T) {
	srv := Service{backend: &fakeService{status: service.StatusRunning, startErr: errors.New("start failed")}}

	if err := srv.Restart(); err == nil {
		t.Fatal("Restart err = nil, want an error")
	}
}
