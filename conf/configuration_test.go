package conf

import (
	"encoding/hex"
	"math"
	"os"
	"testing"
	"time"

	"gitlab.com/NebulousLabs/fastrand"
)

// TestLoadConfig ensures that LoadConfig works as expected.
func TestLoadConfig(t *testing.T) {
	envVarsReq := []string{
		"SERVER_DOMAIN",
		"SKYNET_DB_USER",
		"SKYNET_DB_PASS",
		"SKYNET_DB_HOST",
		"SKYNET_DB_PORT",
		"SIA_API_PASSWORD",
	}
	envVarsOpt := []string{
		"SKYNET_ACCOUNTS_HOST",
		"SKYNET_ACCOUNTS_PORT",
		"PINNER_LOG_FILE",
		"PINNER_LOG_LEVEL",
		"PINNER_SLEEP_BETWEEN_SCANS",
		"API_HOST",
		"API_PORT",
	}
	envVars := append(envVarsReq, envVarsOpt...)
	// Store all env var values.
	values := make(map[string]string)
	for _, key := range envVars {
		val, exists := os.LookupEnv(key)
		if exists {
			values[key] = val
		}
	}
	// Set all required vars, so the test will pass even if the environment is
	// not fully set.
	for _, key := range envVarsReq {
		err := os.Setenv(key, key+"value")
		if err != nil {
			t.Fatal(err)
		}
	}
	// Unset all optional vars.
	for _, key := range envVarsOpt {
		err := os.Unsetenv(key)
		if err != nil {
			t.Fatal(err)
		}
	}
	// Set them back up at the end of the test.
	defer func(vals map[string]string) {
		for _, key := range envVars {
			val, exists := vals[key]
			if exists {
				err := os.Setenv(key, val)
				if err != nil {
					t.Error(err)
				}
			} else {
				err := os.Unsetenv(key)
				if err != nil {
					t.Error(err)
				}
			}
		}
	}(values)
	// Get the values without setting any optionals.
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	// Ensure the required ones match the environment.
	if cfg.ServerName != os.Getenv("SERVER_DOMAIN") {
		t.Fatal("Bad SERVER_DOMAIN")
	}
	if cfg.DBCredentials.User != os.Getenv("SKYNET_DB_USER") {
		t.Fatal("Bad SKYNET_DB_USER")
	}
	if cfg.DBCredentials.Password != os.Getenv("SKYNET_DB_PASS") {
		t.Fatal("Bad SKYNET_DB_PASS")
	}
	if cfg.DBCredentials.Host != os.Getenv("SKYNET_DB_HOST") {
		t.Fatal("Bad SKYNET_DB_HOST")
	}
	if cfg.DBCredentials.Port != os.Getenv("SKYNET_DB_PORT") {
		t.Fatal("Bad SKYNET_DB_PORT")
	}
	if cfg.SiaAPIPassword != os.Getenv("SIA_API_PASSWORD") {
		t.Fatal("Bad SIA_API_PASSWORD")
	}
	// Ensure the optional ones have their default values.
	if cfg.AccountsHost != defaultAccountsHost {
		t.Fatal("Bad AccountsHost")
	}
	if cfg.AccountsPort != defaultAccountsPort {
		t.Fatal("Bad AccountsPort")
	}
	if cfg.LogFile != defaultLogFile {
		t.Fatal("Bad LogFile")
	}
	if cfg.LogLevel != defaultLogLevel {
		t.Fatal("Bad LogLevel")
	}
	if cfg.SleepBetweenScans != 0 {
		t.Fatal("Bad SleepBetweenScans")
	}
	if cfg.SiaAPIHost != defaultSiaAPIHost {
		t.Fatal("Bad SiaAPIHost")
	}
	if cfg.SiaAPIPort != defaultSiaAPIPort {
		t.Fatal("Bad SiaAPIPort")
	}

	// Set the optionals to custom values.
	optionalValues := make(map[string]string)
	for _, key := range envVarsOpt {
		optionalValues[key] = hex.EncodeToString(fastrand.Bytes(16))
		err = os.Setenv(key, optionalValues[key])
		if err != nil {
			t.Fatal(err)
		}
	}
	// We'll set a special value for PINNER_SLEEP_BETWEEN_SCANS because it needs
	// to be a valid duration string.
	optionalValues["PINNER_SLEEP_BETWEEN_SCANS"] = time.Duration(fastrand.Intn(math.MaxInt)).String()
	err = os.Setenv("PINNER_SLEEP_BETWEEN_SCANS", optionalValues["PINNER_SLEEP_BETWEEN_SCANS"])
	if err != nil {
		t.Fatal(err)
	}
	// Load the config again.
	cfg, err = LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	// Ensure all optionals got the custom values we set for them.
	if cfg.AccountsHost != optionalValues["SKYNET_ACCOUNTS_HOST"] {
		t.Fatal("Bad AccountsHost")
	}
	if cfg.AccountsPort != optionalValues["SKYNET_ACCOUNTS_PORT"] {
		t.Fatal("Bad AccountsPort")
	}
	if cfg.LogFile != optionalValues["PINNER_LOG_FILE"] {
		t.Fatal("Bad LogFile")
	}
	if cfg.LogLevel.String() != optionalValues["PINNER_LOG_LEVEL"] {
		t.Fatal("Bad LogLevel")
	}
	if tm, err := time.ParseDuration(optionalValues["PINNER_SLEEP_BETWEEN_SCANS"]); err != nil || cfg.SleepBetweenScans != tm {
		t.Fatal("Bad SleepBetweenScans")
	}
	if cfg.SiaAPIHost != optionalValues["API_HOST"] {
		t.Fatal("Bad SiaAPIHost")
	}
	if cfg.SiaAPIPort != optionalValues["API_PORT"] {
		t.Fatal("Bad SiaAPIPort")
	}
}
