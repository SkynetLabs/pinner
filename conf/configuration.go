package conf

import (
	"os"

	"github.com/joho/godotenv"
	"gitlab.com/NebulousLabs/errors"
)

var (
	// AccountsHost defines the IP or hostname of the local accounts service.
	AccountsHost = "10.10.10.70"
	// AccountsPort defines the port of the local accounts service.
	AccountsPort = "3000"
	// DBUser username for connecting to the database.
	DBUser string
	// DBPassword password for connecting to the database.
	DBPassword string
	// DBHost host for connecting to the database.
	DBHost string
	// DBPort port for connecting to the database.
	DBPort string
	// LogLevel defines the logging level of the entire service.
	LogLevel = "info"
	// ServerName holds the name of the current server. This name will be used
	// for identifying which servers are pinning a given skylink.
	ServerName string
	// SiaAPIPassword is the apipassword for the local skyd
	SiaAPIPassword string
	// SiaAPIHost is the hostname/IP of the local skyd
	SiaAPIHost = "10.10.10.10"
	// SiaAPIPort is the port of the local skyd
	SiaAPIPort = "9980"
)

// LoadConfiguration loads the required service configuration from the
// environment and the provided .env file.
func LoadConfiguration() error {
	// Load the environment variables from the .env file.
	// Existing variables take precedence and won't be overwritten.
	_ = godotenv.Load()

	var ok bool
	var val string

	// Required
	if ServerName, ok = os.LookupEnv("SERVER_DOMAIN"); !ok {
		return errors.New("missing env var SERVER_DOMAIN")
	}
	if DBUser, ok = os.LookupEnv("SKYNET_DB_USER"); !ok {
		return errors.New("missing env var SKYNET_DB_USER")
	}
	if DBPassword, ok = os.LookupEnv("SKYNET_DB_PASS"); !ok {
		return errors.New("missing env var SKYNET_DB_PASS")
	}
	if DBHost, ok = os.LookupEnv("SKYNET_DB_HOST"); !ok {
		return errors.New("missing env var SKYNET_DB_HOST")
	}
	if DBPort, ok = os.LookupEnv("SKYNET_DB_PORT"); !ok {
		return errors.New("missing env var SKYNET_DB_PORT")
	}
	if SiaAPIPassword, ok = os.LookupEnv("SIA_API_PASSWORD"); !ok {
		return errors.New("missing env var SIA_API_PASSWORD")
	}

	// Optional
	if val, ok = os.LookupEnv("SKYNET_ACCOUNTS_HOST"); ok {
		AccountsHost = val
	}
	if val, ok = os.LookupEnv("SKYNET_ACCOUNTS_PORT"); ok {
		AccountsPort = val
	}
	if val, ok = os.LookupEnv("PINNER_LOG_LEVEL"); ok {
		LogLevel = val
	}
	if val, ok = os.LookupEnv("API_HOST"); ok {
		SiaAPIHost = val
	}
	if val, ok = os.LookupEnv("API_PORT"); ok {
		SiaAPIPort = val
	}

	return nil
}
