# These variables get inserted into ./build/commit.go
BUILD_TIME=$(shell date -u)
GIT_REVISION=$(shell git rev-parse --short HEAD)
GIT_DIRTY=$(shell git diff-index --quiet HEAD -- || echo "✗-")

ldflags= -X "github.com/skynetlabs/pinner/build.GitRevision=${GIT_DIRTY}${GIT_REVISION}" \
-X "github.com/skynetlabs/pinner/build.BuildTime=${BUILD_TIME}"

racevars= history_size=3 halt_on_error=1 atexit_sleep_ms=2000

# all will build and install release binaries
all: release

# count says how many times to run the tests.
count = 1
# pkgs changes which packages the makefile calls operate on. run changes which
# tests are run during testing.
pkgs = ./ ./api ./conf ./database ./logger ./skyd ./test ./workers

# integration-pkgs defines the packages which contain integration tests
integration-pkgs = ./test ./test/api ./test/database

# run determines which tests run when running any variation of 'make test'.
run = .

# fmt calls go fmt on all packages.
fmt:
	gofmt -s -l -w $(pkgs)

# vet calls go vet on all packages.
# We don't check composite literals because we need to use unkeyed fields for
# MongoDB's BSONs and that sets vet off.
# NOTE: go vet requires packages to be built in order to obtain type info.
vet:
	go vet -composites=false $(pkgs)

# markdown-spellcheck runs codespell on all markdown files that are not
# vendored.
markdown-spellcheck:
	pip install codespell 1>/dev/null 2>&1
	git ls-files "*.md" :\!:"vendor/**" | xargs codespell --check-filenames

# lint runs golangci-lint (which includes revive, a spellcheck of the codebase,
# and other linters), the custom analyzers, and also a markdown spellchecker.
lint: fmt markdown-spellcheck vet
	golangci-lint run -c .golangci.yml
	go mod tidy
	analyze -lockcheck -- $(pkgs)

# lint-ci runs revive.
lint-ci:
# revive is skipped on Windows.
ifneq ("$(OS)","Windows_NT")
# Linux
	go install github.com/mgechev/revive@latest
	revive -set_exit_status $(pkgs)
	go mod tidy
endif

# Credentials and port we are going to use for our test MongoDB instance.
MONGO_USER=admin
MONGO_PASSWORD=aO4tV5tC1oU3oQ7u
MONGO_PORT=17018

# call_mongo is a helper function that executes a query in an `eval` call to the
# test mongo instance.
define call_mongo
    docker exec pinner-mongo-test-db mongo -u $(MONGO_USER) -p $(MONGO_PASSWORD) --port $(MONGO_PORT) --eval $(1)
endef

# start-mongo starts a local mongoDB container with no persistence.
# We first prepare for the start of the container by making sure the test
# keyfile has the right permissions, then we clear any potential leftover
# containers with the same name. After we start the container we initialise a
# single node replica set. All the output is discarded because it's noisy and
# if it causes a failure we'll immediately know where it is even without it.
start-mongo:
	-docker stop pinner-mongo-test-db 1>/dev/null 2>&1
	-docker rm pinner-mongo-test-db 1>/dev/null 2>&1
	chmod 400 $(shell pwd)/test/fixtures/mongo_keyfile
	docker run \
     --rm \
     --detach \
     --name pinner-mongo-test-db \
     -p $(MONGO_PORT):$(MONGO_PORT) \
     -e MONGO_INITDB_ROOT_USERNAME=$(MONGO_USER) \
     -e MONGO_INITDB_ROOT_PASSWORD=$(MONGO_PASSWORD) \
     -v $(shell pwd)/test/fixtures/mongo_keyfile:/data/mgkey \
	mongo:4.4.1 mongod --port=$(MONGO_PORT) --replSet=skynet --keyFile=/data/mgkey 1>/dev/null 2>&1
	# wait for mongo to start before we try to configure it
	status=1 ; while [[ $$status -gt 0 ]]; do \
		sleep 1 ; \
		$(call call_mongo,"") 1>/dev/null 2>&1 ; \
		status=$$? ; \
	done
	# Initialise a single node replica set.
	$(call call_mongo,"rs.initiate({_id: \"skynet\", members: [{ _id: 0, host: \"localhost:$(MONGO_PORT)\" }]})") 1>/dev/null 2>&1

stop-mongo:
	-docker stop pinner-mongo-test-db

# debug builds and installs debug binaries. This will also install the utils.
debug:
	go install -tags='debug profile netgo' -ldflags='$(ldflags)' $(pkgs)
debug-race:
	GORACE='$(racevars)' go install -race -tags='debug profile netgo' -ldflags='$(ldflags)' $(pkgs)

# dev builds and installs developer binaries. This will also install the utils.
dev:
	go install -tags='dev debug profile netgo' -ldflags='$(ldflags)' $(pkgs)
dev-race:
	GORACE='$(racevars)' go install -race -tags='dev debug profile netgo' -ldflags='$(ldflags)' $(pkgs)

# release builds and installs release binaries.
release:
	go install -tags='netgo' -ldflags='-s -w $(ldflags)' $(release-pkgs)
release-race:
	GORACE='$(racevars)' go install -race -tags='netgo' -ldflags='-s -w $(ldflags)' $(release-pkgs)
release-util:
	go install -tags='netgo' -ldflags='-s -w $(ldflags)' $(release-pkgs) $(util-pkgs)

# check is a development helper that ensures all test files at least build
# without actually running the tests.
check:
	go test --exec=true ./...

bench: fmt
	go test -tags='debug testing netgo' -timeout=500s -run=XXX -bench=. $(pkgs) -count=$(count)

test:
	go test -short -tags='debug testing netgo' -timeout=5s $(pkgs) -run=. -count=$(count)

# Tests in this group may rely on external services (such as MongoDB).
test-long: lint lint-ci start-mongo
	@mkdir -p cover
	GORACE='$(racevars)' go test -race --coverprofile='./cover/cover.out' -v -failfast -tags='testing debug netgo' -timeout=60s $(pkgs) -run=$(run) -count=$(count)
	GORACE='$(racevars)' go test -race --coverprofile='./cover/cover.out' -v -tags='testing debug netgo' -timeout=600s $(integration-pkgs) -run=$(run) -count=$(count)
	-make stop-mongo

run-dev:
	go run -tags="dev" .

.PHONY: all fmt install release check test test-long start-mongo stop-mongo run-dev
