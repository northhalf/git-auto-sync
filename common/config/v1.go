package config

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/kirsle/configdir"
	"github.com/ztrue/tracerr"
)

type ConfigV1 struct {
	Repos []string `json:"repos"`
	Envs  []string `json:"envs"`
}

// @description    Reads version-one daemon configuration.
//
// ReadV1 creates the local configuration directory and decodes config.json, returning an empty
// configuration when the file does not exist. A deferred close error becomes the returned error
// when no earlier error occurred.
//
// @return          *ConfigV1  "decoded configuration, or an empty configuration when no file exists"
//
// @return          err        "nil on success, or an error creating, opening, decoding, or closing the file"
func ReadV1() (_ *ConfigV1, err error) {
	configPath := configdir.LocalConfig("git-auto-sync")
	err = configdir.MakePath(configPath)
	if err != nil {
		return nil, tracerr.Wrap(err)
	}

	configFile := filepath.Join(configPath, "config.json")
	config := ConfigV1{}

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
// WriteV1 creates the local configuration directory and replaces config.json with the encoded
// configuration. A deferred close error becomes the returned error when no earlier error occurred.
//
// @param           config  "version-one configuration to persist"
//
// @return          err     "nil on success, or an error creating, encoding, or closing the file"
func WriteV1(config *ConfigV1) (err error) {
	configPath := configdir.LocalConfig("git-auto-sync")
	err = configdir.MakePath(configPath)
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
