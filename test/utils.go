package test

import (
	"bytes"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/skynetlabs/pinner/conf"
	"github.com/skynetlabs/pinner/database"
	"gitlab.com/NebulousLabs/errors"
	"gitlab.com/NebulousLabs/fastrand"
	"gitlab.com/SkynetLabs/skyd/skymodules"
	"go.sia.tech/siad/crypto"
)

var (
	// ServerName is what we'll use for ServerName during testing. We want
	// to have it in a separate variable, so we can set it in different tests
	// without worrying about them choosing different names.
	ServerName = "test.server.name"
	// confMu is a mutex which ensures that no two threads are
	// going to mutate the configuration environment variables at the same time.
	// This is done, so we can always restore the environment to the state
	// before the intervention.
	confMu sync.Mutex
)

type (
	// ResponseWriter is a testing ResponseWriter implementation.
	ResponseWriter struct {
		Buffer bytes.Buffer
		Status int
	}
)

// Header implementation.
func (w ResponseWriter) Header() http.Header {
	return http.Header{}
}

// Write implementation.
func (w ResponseWriter) Write(b []byte) (int, error) {
	return w.Buffer.Write(b)
}

// WriteHeader implementation.
func (w ResponseWriter) WriteHeader(statusCode int) {
	w.Status = statusCode
}

// DBNameForTest sanitizes the input string, so it can be used as an email or
// sub.
func DBNameForTest(s string) string {
	return strings.ReplaceAll(s, "/", "_")
}

// DBTestCredentials sets the environment variables to what we have defined in Makefile.
func DBTestCredentials() database.DBCredentials {
	return database.DBCredentials{
		User:     "admin",
		Password: "aO4tV5tC1oU3oQ7u",
		Host:     "localhost",
		Port:     "17018",
	}
}

// LoadTestConfig temporarily replaces environment variables with their
// test values, loads the configuration with these test values and then restores
// the original environment.
func LoadTestConfig() (conf.Config, error) {
	confMu.Lock()
	defer confMu.Unlock()
	envVars := []string{
		"SERVER_DOMAIN",
		"SKYNET_DB_USER",
		"SKYNET_DB_PASS",
		"SKYNET_DB_HOST",
		"SKYNET_DB_PORT",
		"SIA_API_PASSWORD",
	}
	// Store the original values.
	originals := make(map[string]string)
	for _, ev := range envVars {
		val, ok := os.LookupEnv(ev)
		if ok {
			originals[ev] = val
		}
	}
	// Ensure these will be restored before we return and unlock.
	defer func() {
		for _, ev := range envVars {
			val, ok := originals[ev]
			if ok {
				os.Setenv(ev, val)
			} else {
				os.Unsetenv(ev)
			}
		}
	}()
	// Set the test values we need.
	dbcr := DBTestCredentials()
	e1 := os.Setenv("SERVER_DOMAIN", ServerName)
	e2 := os.Setenv("SKYNET_DB_USER", dbcr.User)
	e3 := os.Setenv("SKYNET_DB_PASS", dbcr.Password)
	e4 := os.Setenv("SKYNET_DB_HOST", dbcr.Host)
	e5 := os.Setenv("SKYNET_DB_PORT", dbcr.Port)
	e6 := os.Setenv("SIA_API_PASSWORD", "testSiaApiPassword")
	if err := errors.Compose(e1, e2, e3, e4, e5, e6); err != nil {
		return conf.Config{}, err
	}
	return conf.LoadConfig()
}

// RandomSkylink generates a random skylink
func RandomSkylink() string {
	var h crypto.Hash
	fastrand.Read(h[:])
	sl, _ := skymodules.NewSkylinkV1(h, 0, 0)
	return sl.String()
}
