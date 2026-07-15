package config

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/ztrue/tracerr"
)

type DaemonConfig struct {
	Repos []string `json:"repos"`
	Envs  []string `json:"envs"`
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
	configDir, err := os.UserConfigDir()
	if err != nil {
		return nil, tracerr.Wrap(err)
	}

	configPath := filepath.Join(configDir, "git-auto-sync")
	err = os.MkdirAll(configPath, 0o700)
	if err != nil {
		return nil, tracerr.Wrap(err)
	}

	configFile := filepath.Join(configPath, "config.json")
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
	configDir, err := os.UserConfigDir()
	if err != nil {
		return tracerr.Wrap(err)
	}

	configPath := filepath.Join(configDir, "git-auto-sync")
	err = os.MkdirAll(configPath, 0o700)
	if err != nil {
		return tracerr.Wrap(err)
	}

	configFile := filepath.Join(configPath, "config.json")

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
