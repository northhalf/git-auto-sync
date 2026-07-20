package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/ztrue/tracerr"
)

// Default synchronization and debounce intervals and the default Git executable. Repository-local
// [auto-sync] settings override global settings, and global settings override these defaults. Time
// units are minutes.
const (
	DefaultSyncInterval = 60 * time.Minute // default: sync once per hour
	DefaultDebounce     = 10 * time.Minute // default: filesystem event debounce
	DefaultGitExec      = "git"            // default: resolve git through PATH
)

// Settings is the complete view of the platform config.json. It carries daemon state (Repos, Envs)
// and the three global synchronization settings. The same type is reused as the per-repository
// view when reading [auto-sync]; in that role only SyncInterval, Debounce, and GitExec may be
// non-nil. Pointer fields use nil to mean unset.
type Settings struct {
	Repos        []string `json:"repos"`
	Envs         []string `json:"envs"`
	SyncInterval *int     `json:"syncInterval,omitempty"` // minutes
	Debounce     *int     `json:"debounce,omitempty"`     // minutes
	GitExec      *string  `json:"gitexec,omitempty"`
}

// @description    Resolves the global settings file path.
//
// settingsFile joins the platform configuration directory with the git-auto-sync directory and
// config.json file name. It does not create the directory so callers that only inspect the path,
// such as the modification-time poller, avoid filesystem side effects.
//
// @return          string  "absolute path to the global settings file"
//
// @return          error   "nil on success, or an error when the platform config directory cannot be resolved"
func settingsFile() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", tracerr.Wrap(err)
	}

	return filepath.Join(configDir, "git-auto-sync", "config.json"), nil
}

// @description    Reports the global settings modification time.
//
// GlobalSettingsModTime stats the global settings file and returns its modification time. It
// returns the zero time with a nil error when the file does not yet exist so callers can treat a
// missing file as an empty, unchanged configuration.
//
// @return          time.Time  "file modification time, or the zero time when the file is absent"
//
// @return          error      "nil on success or when the file is absent, or an error resolving or stating the path"
func GlobalSettingsModTime() (time.Time, error) {
	configFile, err := settingsFile()
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

// @description    Reads global settings.
//
// ReadGlobalSettings creates the local configuration directory and decodes config.json, returning
// an empty configuration when the file does not exist. A corrupt or unreadable existing file is a
// hard error. A deferred close error becomes the returned error when no earlier error occurred.
//
// @return          *Settings  "decoded settings, or an empty settings when no file exists"
//
// @return          err        "nil on success, or an error creating, opening, decoding, or closing the file"
func ReadGlobalSettings() (_ *Settings, err error) {
	configFile, err := settingsFile()
	if err != nil {
		return nil, tracerr.Wrap(err)
	}

	if mkErr := os.MkdirAll(filepath.Dir(configFile), 0o700); mkErr != nil {
		return nil, tracerr.Wrap(mkErr)
	}

	settings := Settings{}

	if _, err = os.Stat(configFile); os.IsNotExist(err) {
		return &settings, nil
	} else {
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
		err = decoder.Decode(&settings)
		if err != nil {
			return nil, tracerr.Wrap(err)
		}
	}

	return &settings, nil
}

// @description    Writes global settings.
//
// WriteGlobalSettings creates the local configuration directory and replaces config.json with the
// encoded settings. A deferred close error becomes the returned error when no earlier error
// occurred.
//
// @param           settings  "settings to persist"
//
// @return          err       "nil on success, or an error creating, encoding, or closing the file"
func WriteGlobalSettings(settings *Settings) (err error) {
	configFile, err := settingsFile()
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
	err = encoder.Encode(settings)
	if err != nil {
		return tracerr.Wrap(err)
	}

	return nil
}

// SettingField accesses one Settings field through its configuration key.
type SettingField struct {
	// Decode parses a raw value and assigns it to the field.
	Decode func(s *Settings, value string) error
	// Clear resets the field to unset.
	Clear func(s *Settings)
	// Raw returns the field's stored string form and whether it is set.
	Raw func(s *Settings) (string, bool)
}

// SettingKeys maps each configuration key to the accessors of the Settings field that holds its
// value. It is the single source of truth for the key names the CLI and the local [auto-sync]
// helpers read and write.
var SettingKeys = map[string]SettingField{
	"syncInterval": {
		Decode: func(s *Settings, v string) error {
			n, err := strconv.Atoi(v)
			if err != nil {
				return tracerr.Wrap(err)
			}
			s.SyncInterval = &n
			return nil
		},
		Clear: func(s *Settings) { s.SyncInterval = nil },
		Raw: func(s *Settings) (string, bool) {
			if s.SyncInterval == nil {
				return "", false
			}
			return strconv.Itoa(*s.SyncInterval), true
		},
	},
	"debounce": {
		Decode: func(s *Settings, v string) error {
			n, err := strconv.Atoi(v)
			if err != nil {
				return tracerr.Wrap(err)
			}
			s.Debounce = &n
			return nil
		},
		Clear: func(s *Settings) { s.Debounce = nil },
		Raw: func(s *Settings) (string, bool) {
			if s.Debounce == nil {
				return "", false
			}
			return strconv.Itoa(*s.Debounce), true
		},
	},
	"gitexec": {
		Decode: func(s *Settings, v string) error {
			s.GitExec = &v
			return nil
		},
		Clear: func(s *Settings) { s.GitExec = nil },
		Raw: func(s *Settings) (string, bool) {
			if s.GitExec == nil {
				return "", false
			}
			return *s.GitExec, true
		},
	},
}

// @description    Reads repository-local [auto-sync] settings.
//
// ReadLocalSettings opens the repository at repoPath, reads the [auto-sync] section, and decodes
// the syncInterval, debounce, and gitexec keys into a Settings. Unset keys leave their fields nil.
// Repos and Envs are always nil in the returned value.
//
// @param           repoPath    "path to the repository root"
//
// @return          *Settings   "local settings; only SyncInterval, Debounce, and GitExec may be set"
//
// @return          error       "nil on success, or an error opening the repository or reading its config"
func ReadLocalSettings(repoPath string) (*Settings, error) {
	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		return nil, tracerr.Wrap(err)
	}

	cfg, err := repo.Config()
	if err != nil {
		return nil, tracerr.Wrap(err)
	}

	section := cfg.Raw.Section("auto-sync")
	settings := &Settings{}
	for key, field := range SettingKeys {
		value := section.Option(key)
		if value == "" {
			continue
		}
		if err := field.Decode(settings, value); err != nil {
			return nil, tracerr.Wrap(err)
		}
	}
	return settings, nil
}

// @description    Sets a repository-local [auto-sync] key.
//
// SetLocalSetting opens the repository at repoPath, sets key to value in the [auto-sync] section,
// and persists the config. It does not validate value; callers validate before calling.
//
// @param           repoPath  "path to the repository root"
//
// @param           key       "one of syncInterval, debounce, gitexec"
//
// @param           value     "value to store"
//
// @return          error     "nil on success, or an error opening, updating, or persisting the config"
func SetLocalSetting(repoPath, key, value string) error {
	if _, ok := SettingKeys[key]; !ok {
		return tracerr.Errorf("unknown auto-sync key: %s", key)
	}

	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		return tracerr.Wrap(err)
	}

	cfg, err := repo.Config()
	if err != nil {
		return tracerr.Wrap(err)
	}

	cfg.Raw.Section("auto-sync").SetOption(key, value)
	return repo.SetConfig(cfg)
}

// @description    Removes a repository-local [auto-sync] key.
//
// UnsetLocalSetting opens the repository at repoPath, removes key from the [auto-sync] section,
// and persists the config. Removing an absent key is a no-op.
//
// @param           repoPath  "path to the repository root"
//
// @param           key       "one of syncInterval, debounce, gitexec"
//
// @return          error     "nil on success, or an error opening, updating, or persisting the config"
func UnsetLocalSetting(repoPath, key string) error {
	if _, ok := SettingKeys[key]; !ok {
		return tracerr.Errorf("unknown auto-sync key: %s", key)
	}

	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		return tracerr.Wrap(err)
	}

	cfg, err := repo.Config()
	if err != nil {
		return tracerr.Wrap(err)
	}

	cfg.Raw.Section("auto-sync").RemoveOption(key)
	return repo.SetConfig(cfg)
}

// @description    Merges global and local settings into resolved values.
//
// Resolve applies the chain local over global over default and converts minute integers to
// time.Duration. It is a pure function with no side effects. nil inputs are treated as fully
// unset.
//
// @param           global       "global settings, or nil"
//
// @param           local        "repository-local settings, or nil"
//
// @return          time.Duration  "resolved sync interval"
//
// @return          time.Duration  "resolved debounce"
//
// @return          string         "resolved git executable"
func Resolve(global, local *Settings) (syncInterval, debounce time.Duration, gitExec string) {
	syncMinutes := DefaultSyncInterval / time.Minute
	if local != nil && local.SyncInterval != nil {
		syncMinutes = time.Duration(*local.SyncInterval)
	} else if global != nil && global.SyncInterval != nil {
		syncMinutes = time.Duration(*global.SyncInterval)
	}

	debounceMinutes := DefaultDebounce / time.Minute
	if local != nil && local.Debounce != nil {
		debounceMinutes = time.Duration(*local.Debounce)
	} else if global != nil && global.Debounce != nil {
		debounceMinutes = time.Duration(*global.Debounce)
	}

	gitExec = DefaultGitExec
	if local != nil && local.GitExec != nil {
		gitExec = *local.GitExec
	} else if global != nil && global.GitExec != nil {
		gitExec = *global.GitExec
	}

	return syncMinutes * time.Minute, debounceMinutes * time.Minute, gitExec
}

// @description    Produces a stable fingerprint of the three synchronization settings.
//
// LocalFingerprint returns a comparable string that changes only when syncInterval, debounce, or
// gitexec changes. The daemon's change detectors use it to avoid restarting watchers when unrelated
// configuration content changes: the per-repository poller compares fingerprints of [auto-sync]
// settings, and the global poller compares fingerprints of the daemon-wide settings.
//
// @param           s       "settings to fingerprint, or nil"
//
// @return          string  "fingerprint of the three synchronization settings"
func LocalFingerprint(s *Settings) string {
	if s == nil {
		s = &Settings{}
	}
	sync := ""
	if s.SyncInterval != nil {
		sync = strconv.Itoa(*s.SyncInterval)
	}
	debounce := ""
	if s.Debounce != nil {
		debounce = strconv.Itoa(*s.Debounce)
	}
	git := ""
	if s.GitExec != nil {
		git = *s.GitExec
	}
	return "syncInterval=" + sync + "|debounce=" + debounce + "|gitexec=" + git
}
