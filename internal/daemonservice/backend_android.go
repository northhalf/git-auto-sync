//go:build android && arm64

package daemonservice

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kardianos/service"

	"github.com/northhalf/git-auto-sync/internal/termux"
)

// @description    Creates the Termux runit service-manager backend.
//
// @param           daemon  "unused lifecycle implementation retained for constructor parity"
//
// @return          serviceBackend  "validated Termux runit controller"
//
// @return          error           "nil on success, or a Termux dependency or executable error"
func newServiceBackend(daemon service.Interface) (serviceBackend, error) {
	_ = daemon
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	programPath, err := termux.ProgramPath(executable, os.Args)
	if err != nil {
		return nil, err
	}
	daemonPath := filepath.Join(filepath.Dir(programPath), daemonServiceName)
	return newRunitBackend(os.Getenv("PREFIX"), daemonPath, runSVCommand)
}

// @description    Executes an absolute Termux sv command and preserves diagnostic output.
//
// runSVCommand routes the exec through termux.Command because Termux ships sv as a static
// PIE binary, which the Android kernel refuses to execve directly from a Go process.
//
// @param           path  "absolute sv executable path"
//
// @param           args  "sv operation and absolute service directory"
//
// @return          []byte  "combined standard output and standard error"
//
// @return          error   "nil on success, or a command error containing diagnostic output"
func runSVCommand(path string, args ...string) ([]byte, error) {
	output, err := termux.Command(path, args...).CombinedOutput()
	if err != nil {
		return output, fmt.Errorf("sv command failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return output, nil
}
