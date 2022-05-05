package conf

import (
	"os"

	"github.com/joho/godotenv"
	"github.com/skynetlabs/pinner/database"
	"gitlab.com/NebulousLabs/errors"
)

// Default configuration values.
// For individual descriptions see Config.
const (
	defaultAccountsHost = "10.10.10.70"
	defaultAccountsPort = "3000"
	defaultLogLevel     = "info"
	defaultSiaAPIHost   = "10.10.10.10"
	defaultSiaAPIPort   = "9980"
	defaultMinPinners   = 1
)

type (
	// Config represents the entire configurable state of the service. If a
	// value is not here, then it can't be configured.
	Config struct {
		// AccountsHost defines the IP or hostname of the local accounts service.
		AccountsHost string
		// AccountsPort defines the port of the local accounts service.
		AccountsPort string
		// DBCredentials holds all the information we need to connect to the DB.
		DBCredentials database.DBCredentials
		// LogLevel defines the logging level of the entire service.
		LogLevel string
		// MinPinners defines the minimum number of pinning servers
		// which a skylink needs in order to not be considered underpinned.
		// Anything below this value requires more servers to pin the skylink.
		MinPinners int
		// ServerName holds the name of the current server. This name will be
		// used for identifying which servers are pinning a given skylink.
		ServerName string
		// SiaAPIPassword is the apipassword for the local skyd
		SiaAPIPassword string
		// SiaAPIHost is the hostname/IP of the local skyd
		SiaAPIHost string
		// SiaAPIPort is the port of the local skyd
		SiaAPIPort string
	}
)

// LoadConfig loads the required service defaultConfig from the environment and
// the provided .env file.
func LoadConfig() (Config, error) {
	// Load the environment variables from the .env file.
	// Existing variables take precedence and won't be overwritten.
	_ = godotenv.Load()

	// Start with the default values.
	cfg := Config{
		AccountsHost:  defaultAccountsHost,
		AccountsPort:  defaultAccountsPort,
		DBCredentials: database.DBCredentials{},
		LogLevel:      defaultLogLevel,
		MinPinners:    defaultMinPinners,
		SiaAPIHost:    defaultSiaAPIHost,
		SiaAPIPort:    defaultSiaAPIPort,
	}

	var ok bool
	var val string

	// Required
	if cfg.ServerName, ok = os.LookupEnv("SERVER_DOMAIN"); !ok {
		return Config{}, errors.New("missing env var SERVER_DOMAIN")
	}
	if cfg.DBCredentials.User, ok = os.LookupEnv("SKYNET_DB_USER"); !ok {
		return Config{}, errors.New("missing env var SKYNET_DB_USER")
	}
	if cfg.DBCredentials.Password, ok = os.LookupEnv("SKYNET_DB_PASS"); !ok {
		return Config{}, errors.New("missing env var SKYNET_DB_PASS")
	}
	if cfg.DBCredentials.Host, ok = os.LookupEnv("SKYNET_DB_HOST"); !ok {
		return Config{}, errors.New("missing env var SKYNET_DB_HOST")
	}
	if cfg.DBCredentials.Port, ok = os.LookupEnv("SKYNET_DB_PORT"); !ok {
		return Config{}, errors.New("missing env var SKYNET_DB_PORT")
	}
	if cfg.SiaAPIPassword, ok = os.LookupEnv("SIA_API_PASSWORD"); !ok {
		return Config{}, errors.New("missing env var SIA_API_PASSWORD")
	}

	// Optional
	if val, ok = os.LookupEnv("SKYNET_ACCOUNTS_HOST"); ok {
		cfg.AccountsHost = val
	}
	if val, ok = os.LookupEnv("SKYNET_ACCOUNTS_PORT"); ok {
		cfg.AccountsPort = val
	}
	if val, ok = os.LookupEnv("PINNER_LOG_LEVEL"); ok {
		cfg.LogLevel = val
	}
	if val, ok = os.LookupEnv("API_HOST"); ok {
		cfg.SiaAPIHost = val
	}
	if val, ok = os.LookupEnv("API_PORT"); ok {
		cfg.SiaAPIPort = val
	}

	return cfg, nil
}
