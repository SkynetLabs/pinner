package conf

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"
	"github.com/skynetlabs/pinner/database"
	"gitlab.com/NebulousLabs/errors"
	"gitlab.com/SkynetLabs/skyd/build"
	"go.mongodb.org/mongo-driver/mongo"
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

// Cluster-wide configuration variable names.
// Stored in the database.
const (
	// ConfMinPinners holds the name of the configuration setting which defines
	// the minimum number of pinners we want to ensure for each skyfile.
	ConfMinPinners = "min_pinners"
)

const (
	// minPinnersMinValue is the lowest allowed value for the number of pinners
	// we want to be pinning each skylink. We don't go under 1 because if you
	// don't want to ensure that skylinks are being pinned, you shouldn't be
	// running this service in the first place.
	minPinnersMinValue = 1
	// maxPinnersMinValue is the highest allowed value for the number of pinners
	// we want to be pinning each skylink. We want to limit the max number here
	// because raising this number has direct financial consequences for the
	// portal operator. The number 10 was arbitrarily chosen as an acceptable
	// upper bound.
	maxPinnersMinValue = 10
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

// MinPinners returns the cluster-wide value of the minimum number of servers we
// expect to be pinning each skylink.
func MinPinners(ctx context.Context, db *database.DB) (int, error) {
	val, err := db.ConfigValue(ctx, ConfMinPinners)
	if errors.Contains(err, mongo.ErrNoDocuments) {
		return defaultMinPinners, nil
	}
	if err != nil {
		return 0, err
	}
	mp, err := strconv.ParseInt(val, 10, 0)
	if err != nil {
		return 0, err
	}
	if mp < minPinnersMinValue || mp > maxPinnersMinValue {
		errMsg := fmt.Sprintf("Invalid min_pinners value in database configuration! The value must be between %d and %d, it was %v.", mp, minPinnersMinValue, maxPinnersMinValue)
		build.Critical(errMsg)
		return 0, errors.New(errMsg)
	}
	return int(mp), nil
}
