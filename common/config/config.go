package config

type Config = ConfigV1

// @description    Read loads the current daemon configuration through the version-one reader.
//
// @return          *Config  "loaded configuration, or an empty configuration when no file exists"
//
// @return          error    "nil on success, or an error creating, reading, decoding, or closing the file"
func Read() (*Config, error) {
	return ReadV1()
}

// @description    Write persists the current daemon configuration through the version-one writer.
//
// @param           config  "configuration to persist"
//
// @return          error   "nil on success, or an error creating, encoding, or closing the file"
func Write(config *Config) error {
	return WriteV1(config)
}
