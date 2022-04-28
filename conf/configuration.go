package conf

import (
	"os"

	"github.com/joho/godotenv"
	"gitlab.com/NebulousLabs/errors"
)

var (
	// configuration is the current state of the configuration.
	configuration = Config{
		AccountsHost: "10.10.10.70",
		AccountsPort: "3000",
		LogLevel:     "info",
		SiaAPIHost:   "10.10.10.10",
		SiaAPIPort:   "9980",
	}
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

// Conf returns the current state of the configuration of the service.
func Conf() Config {
	return configuration
}

// LoadConf loads the required service configuration from the environment and
// the provided .env file.
func LoadConf() error {
	// Load the environment variables from the .env file.
	// Existing variables take precedence and won't be overwritten.
	_ = godotenv.Load()

	var ok bool
	var val string

	// Required
	if configuration.ServerName, ok = os.LookupEnv("SERVER_DOMAIN"); !ok {
		return errors.New("missing env var SERVER_DOMAIN")
	}
	if configuration.DBUser, ok = os.LookupEnv("SKYNET_DB_USER"); !ok {
		return errors.New("missing env var SKYNET_DB_USER")
	}
	if configuration.DBPassword, ok = os.LookupEnv("SKYNET_DB_PASS"); !ok {
		return errors.New("missing env var SKYNET_DB_PASS")
	}
	if configuration.DBHost, ok = os.LookupEnv("SKYNET_DB_HOST"); !ok {
		return errors.New("missing env var SKYNET_DB_HOST")
	}
	if configuration.DBPort, ok = os.LookupEnv("SKYNET_DB_PORT"); !ok {
		return errors.New("missing env var SKYNET_DB_PORT")
	}
	if configuration.SiaAPIPassword, ok = os.LookupEnv("SIA_API_PASSWORD"); !ok {
		return errors.New("missing env var SIA_API_PASSWORD")
	}

	// Optional
	if val, ok = os.LookupEnv("SKYNET_ACCOUNTS_HOST"); ok {
		configuration.AccountsHost = val
	}
	if val, ok = os.LookupEnv("SKYNET_ACCOUNTS_PORT"); ok {
		configuration.AccountsPort = val
	}
	if val, ok = os.LookupEnv("PINNER_LOG_LEVEL"); ok {
		configuration.LogLevel = val
	}
	if val, ok = os.LookupEnv("API_HOST"); ok {
		configuration.SiaAPIHost = val
	}
	if val, ok = os.LookupEnv("API_PORT"); ok {
		configuration.SiaAPIPort = val
	}

	return nil
}
