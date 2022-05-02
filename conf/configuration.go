package conf

import (
	"os"

	"github.com/joho/godotenv"
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
)

type (
	// Config represents the entire configurable state of the service. If a
	// value is not here, then it can't be configured.
	//
	// AccountsHost defines the IP or hostname of the local accounts service.
	// AccountsPort defines the port of the local accounts service.
	// DBUser username for connecting to the database.
	// DBPassword password for connecting to the database.
	// DBHost host for connecting to the database.
	// DBPort port for connecting to the database.
	// LogLevel defines the logging level of the entire service.
	// ServerName holds the name of the current server. This name will be used
	// 	for identifying which servers are pinning a given skylink.
	// SiaAPIPassword is the apipassword for the local skyd
	// SiaAPIHost is the hostname/IP of the local skyd
	// SiaAPIPort is the port of the local skyd
	Config struct {
		AccountsHost   string
		AccountsPort   string
		DBUser         string
		DBPassword     string
		DBHost         string
		DBPort         string
		LogLevel       string
		ServerName     string
		SiaAPIPassword string
		SiaAPIHost     string
		SiaAPIPort     string
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
		AccountsHost: defaultAccountsHost,
		AccountsPort: defaultAccountsPort,
		LogLevel:     defaultLogLevel,
		SiaAPIHost:   defaultSiaAPIHost,
		SiaAPIPort:   defaultSiaAPIPort,
	}

	var ok bool
	var val string

	// Required
	if cfg.ServerName, ok = os.LookupEnv("SERVER_DOMAIN"); !ok {
		return Config{}, errors.New("missing env var SERVER_DOMAIN")
	}
	if cfg.DBUser, ok = os.LookupEnv("SKYNET_DB_USER"); !ok {
		return Config{}, errors.New("missing env var SKYNET_DB_USER")
	}
	if cfg.DBPassword, ok = os.LookupEnv("SKYNET_DB_PASS"); !ok {
		return Config{}, errors.New("missing env var SKYNET_DB_PASS")
	}
	if cfg.DBHost, ok = os.LookupEnv("SKYNET_DB_HOST"); !ok {
		return Config{}, errors.New("missing env var SKYNET_DB_HOST")
	}
	if cfg.DBPort, ok = os.LookupEnv("SKYNET_DB_PORT"); !ok {
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
