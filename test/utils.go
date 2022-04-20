package test

import (
	"bytes"
	"net/http"
	"strings"

	"github.com/skynetlabs/pinner/database"

	"gitlab.com/NebulousLabs/fastrand"
	"gitlab.com/SkynetLabs/skyd/skymodules"
	"go.sia.tech/siad/crypto"
)

var (
	// TestServerName is what we'll use for ServerName during testing. We want
	// to have it in a separate variable, so we can set it in different tests
	// without worrying about them choosing different names.
	TestServerName = "test.server.name"
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

// RandomSkylink generates a random skylink
func RandomSkylink() string {
	var h crypto.Hash
	fastrand.Read(h[:])
	sl, _ := skymodules.NewSkylinkV1(h, 0, 0)
	return sl.String()
}
