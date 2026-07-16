package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/ztrue/tracerr"
)

type DaemonConfig struct {
	Repos []string `json:"repos"`
	Envs  []string `json:"envs"`
}

// @description    Resolves the daemon configuration file path.
//
// daemonConfigFile joins the platform configuration directory with the git-auto-sync directory
// and config.json file name. It does not create the directory so callers that only inspect the
// path, such as the modification-time poller, avoid filesystem side effects.
//
// @return          string  "absolute path to the daemon configuration file"
//
// @return          error   "nil on success, or an error when the platform config directory cannot be resolved"
func daemonConfigFile() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", tracerr.Wrap(err)
	}

	return filepath.Join(configDir, "git-auto-sync", "config.json"), nil
}

// @description    Reports the daemon configuration modification time.
//
// DaemonConfigModTime stats the daemon configuration file and returns its modification time. It
// returns the zero time with a nil error when the file does not yet exist so callers can treat a
// missing file as an empty, unchanged configuration.
//
// @return          time.Time  "file modification time, or the zero time when the file is absent"
//
// @return          error      "nil on success or when the file is absent, or an error resolving or stating the path"
func DaemonConfigModTime() (time.Time, error) {
	configFile, err := daemonConfigFile()
	if err != nil {
		return time.Time{}, tracerr.Wrap(err)
	}

	info, err := os.Stat(configFile)
	if err != nil {
		if os.IsNotExist(err) {
			return time.Time{}, nil
		}
		return time.Time{}, tracerr.Wrap(err)
	}

	return info.ModTime(), nil
}

// @description    Reads version-one daemon configuration.
//
// Read creates the local configuration directory and decodes config.json, returning an empty
// configuration when the file does not exist. A deferred close error becomes the returned error
// when no earlier error occurred.
//
// @return          *DaemonConfigV1  "decoded configuration, or an empty configuration when no file exists"
//
// @return          err        "nil on success, or an error creating, opening, decoding, or closing the file"
func ReadDaemonConfig() (_ *DaemonConfig, err error) {
	configFile, err := daemonConfigFile()
	if err != nil {
		return nil, tracerr.Wrap(err)
	}

	if mkErr := os.MkdirAll(filepath.Dir(configFile), 0o700); mkErr != nil {
		return nil, tracerr.Wrap(mkErr)
	}

	config := DaemonConfig{}

	if _, err = os.Stat(configFile); os.IsNotExist(err) {
		return &config, nil
	} else {
		// Load the existing file.
		fh, err := os.Open(configFile)
		if err != nil {
			return nil, tracerr.Wrap(err)
		}
		defer func() {
			closeErr := fh.Close()
			if err == nil && closeErr != nil {
				err = tracerr.Wrap(closeErr)
			}
		}()

		decoder := json.NewDecoder(fh)
		err = decoder.Decode(&config)
		if err != nil {
			return nil, tracerr.Wrap(err)
		}
	}

	return &config, nil
}

// @description    Writes version-one daemon configuration.
//
// Write creates the local configuration directory and replaces config.json with the encoded
// configuration. A deferred close error becomes the returned error when no earlier error occurred.
//
// @param           config  "version-one configuration to persist"
//
// @return          err     "nil on success, or an error creating, encoding, or closing the file"
func WriteDaemonConfig(config *DaemonConfig) (err error) {
	configFile, err := daemonConfigFile()
	if err != nil {
		return tracerr.Wrap(err)
	}

	if mkErr := os.MkdirAll(filepath.Dir(configFile), 0o700); mkErr != nil {
		return tracerr.Wrap(mkErr)
	}

	fh, err := os.Create(configFile)
	if err != nil {
		return tracerr.Wrap(err)
	}
	defer func() {
		closeErr := fh.Close()
		if err == nil && closeErr != nil {
			err = tracerr.Wrap(closeErr)
		}
	}()

	encoder := json.NewEncoder(fh)
	err = encoder.Encode(&config)
	if err != nil {
		return tracerr.Wrap(err)
	}

	return nil
}
