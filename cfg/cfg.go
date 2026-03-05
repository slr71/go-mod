package cfg

import (
	"errors"
	"io/fs"
	"os"
	"strings"

	"github.com/knadh/koanf"
	"github.com/knadh/koanf/parsers/dotenv"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
)

// DefaultDotEnvPath is the default path to the dotenv file.
const DefaultDotEnvPath = "/etc/cyverse/de/env/service.env"

// DefaultConfigPath is the default path to the YAML configuration file.
const DefaultConfigPath = "/etc/cyverse/de/configs/service.yml"

// DefaultEnvPrefix is the default environment variable prefix used by Koanf
// when looking up variables in the process's environment.
const DefaultEnvPrefix = "DISCOENV_"

func fileExists(filepath string) (bool, error) {
	_, err := os.Stat(filepath)
	if err == nil {
		return true, nil
	} else if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return false, err
}

type FileType int

const (
	YAML FileType = iota
	JavaProperties
)

// Settings allows for configuration of how a *Koanf instance is created by
// Init().
type Settings struct {
	Delimiter   string   // The delimiter passed to Koanf. Defaults to ".".
	ConfigPath  string   // The path to the YAML config. Defaults to DefaultConfigPath.
	DotEnvPath  string   // The path to the dotenv file. Defaults to DefaultDotEnvPath.
	EnvPrefix   string   // The env var prefix to use for lookups. Defaults to DefaultEnvPrefix.
	StrictMerge bool     // Whether or not to turn on StrictMerge in Koanf. Defaults to false/off.
	FileType    FileType // What kind of file to parse. Defaults to YAML.
}

// Init uses the Settings passed in to set up a *Koanf instance in a way that
// works for the Discovery Environment. The config file should be YAML and is
// used as a baseline configuration.
//
// Config precedence is: yaml < dotenv < environment variables.
func Init(settings *Settings) (*koanf.Koanf, error) {
	var (
		delimiter, configPath, dotEnvPath, envPrefix string
		err                                          error
	)

	if settings == nil {
		settings = &Settings{}
	}

	if settings.Delimiter == "" {
		delimiter = "."
	} else {
		delimiter = settings.Delimiter
	}

	if settings.DotEnvPath == "" {
		dotEnvPath = DefaultDotEnvPath
	} else {
		dotEnvPath = settings.DotEnvPath
	}

	if settings.ConfigPath == "" {
		configPath = DefaultConfigPath
	} else {
		configPath = settings.ConfigPath
	}

	if settings.EnvPrefix == "" {
		envPrefix = DefaultEnvPrefix
	} else {
		envPrefix = settings.EnvPrefix
	}

	k := koanf.NewWithConf(koanf.Conf{
		Delim:       delimiter,
		StrictMerge: settings.StrictMerge,
	})

	var fp koanf.Parser

	switch settings.FileType {
	case YAML:
		fp = yaml.Parser()
	case JavaProperties:
		fp = PropertiesParser()
	default:
		return nil, errors.New("unknown file type")
	}

	// Load from the configuration file as the baseline.
	if err = k.Load(file.Provider(configPath), fp); err != nil {
		return nil, err
	}

	// Don't fail if the dotenv file doesn't exist, just don't load anything
	// from it.
	dotEnvExists, err := fileExists(dotEnvPath)
	if err != nil {
		return nil, err
	}

	if dotEnvExists {
		// We're going to push the environment variables in the dotenv file into
		// the environment so that the koanf's env Provider can deal with them,
		// so use a new koanf instance to avoid polluting the main one.
		envk := koanf.New(".")
		if err = envk.Load(file.Provider(dotEnvPath), dotenv.Parser()); err != nil {
			return nil, err
		}

		// Set the environment variables in the environment, but only if they're
		// not already set. The current environment should take precendence over
		// the dotenv file.
		envKeys := envk.Keys()
		for _, key := range envKeys {
			_, ok := os.LookupEnv(key)
			if !ok {
				val := envk.String(key)
				if err = os.Setenv(key, val); err != nil {
					return nil, err
				}
			}
		}
	}

	// This should deal with all of the environment variables, including those
	// loaded in from the dotenv file.
	if err = k.Load(env.Provider(envPrefix, delimiter, func(s string) string {
		return strings.ReplaceAll(strings.ToLower(strings.TrimPrefix(s, envPrefix)), "_", delimiter)
	}), nil); err != nil {
		return nil, err
	}

	return k, nil
}
