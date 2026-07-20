//go:build android && arm64

package daemonservice

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/kardianos/service"
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
	programPath, err := resolveProgramPath(executable, os.Args, exec.LookPath)
	if err != nil {
		return nil, err
	}
	daemonPath := filepath.Join(filepath.Dir(programPath), runitServiceName)
	return newRunitBackend(os.Getenv("PREFIX"), daemonPath, runSVCommand)
}

// @description    Executes an absolute Termux sv command and preserves diagnostic output.
//
// runSVCommand routes the exec through runWithLinkerRetry because Termux ships sv as a
// static PIE binary, which the Android kernel refuses to execve directly from a Go process.
//
// @param           path  "absolute sv executable path"
//
// @param           args  "sv operation and absolute service directory"
//
// @return          []byte  "combined standard output and standard error"
//
// @return          error   "nil on success, or a command error containing diagnostic output"
func runSVCommand(path string, args ...string) ([]byte, error) {
	selfPath, err := os.Executable()
	if err != nil {
		return nil, err
	}
	output, err := runWithLinkerRetry(selfPath, path, args...)
	if err != nil {
		return output, fmt.Errorf("sv command failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return output, nil
}
