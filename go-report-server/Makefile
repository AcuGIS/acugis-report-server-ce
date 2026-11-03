.PHONY: build build-oss build-cli run test test-oss vet fmt tidy openapi-check openapi-export openapi-sync dist-oss package package-oss deb deb-oss deb-clean verify-deb-oss docker-build-oss docker-push-oss

GO ?= go
BIN ?= bin/jrs-server
CLI_BIN ?= bin/jrs-run-schedule
DISPATCHER_BIN ?= bin/jrs-schedule-dispatcher
OSS_TAGS ?= oss
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo 0.0.0)
# Resolved from the repository root so an OSS export's SOURCE_REVISION file is used.
SOURCE_REVISION ?= $(shell bash -c 'source ../scripts/source-revision.sh && resolve_source_revision "$$(cd .. && pwd)"')
LDFLAGS ?= -X github.com/acugis/acugis-report-server-go/internal/config.DefaultAppVersion=$(VERSION)
DOCKERFILE ?= ../docker/report-server/Dockerfile
DOCKER_CONTEXT ?= ..
# Public tags are the literals in the docker-build/docker-push recipes below.

# This tree is Open Source. Default builds use -tags oss.
build: build-oss

build-oss: openapi-sync
	$(GO) build -tags $(OSS_TAGS) -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/jrs-server
	$(GO) build -tags $(OSS_TAGS) -ldflags "$(LDFLAGS)" -o $(CLI_BIN) ./cmd/jrs-run-schedule
	$(GO) build -tags $(OSS_TAGS) -ldflags "$(LDFLAGS)" -o $(DISPATCHER_BIN) ./cmd/jrs-schedule-dispatcher

build-cli:
	$(GO) build -tags $(OSS_TAGS) -ldflags "$(LDFLAGS)" -o $(CLI_BIN) ./cmd/jrs-run-schedule

run: openapi-sync
	$(GO) run -tags $(OSS_TAGS) ./cmd/jrs-server

test: test-oss

test-oss: openapi-sync
	$(GO) test -tags $(OSS_TAGS) ./cmd/... ./internal/...

openapi-sync:
	$(GO) run ./scripts/sync-openapi.go .

openapi-check: openapi-sync
	$(GO) test -tags $(OSS_TAGS) ./internal/openapi/ ./internal/server/ -run 'OpenAPI|Bundle|Marshal|Deprecated' -count=1

openapi-export: openapi-sync
	$(GO) run ./scripts/export-openapi.go openapi.bundle.yaml

vet:
	$(GO) vet -tags $(OSS_TAGS) ./...

fmt:
	$(GO) fmt ./...

tidy:
	$(GO) mod tidy

# Full native Linux packages (precompiled binaries + runtime tree) live at repo root dist/.
package-oss:
	$(MAKE) -C .. package-oss VERSION=$(VERSION)

package:
	$(MAKE) -C .. package VERSION=$(VERSION)

deb-oss:
	$(MAKE) -C .. deb-oss VERSION=$(VERSION)

deb:
	$(MAKE) -C .. deb VERSION=$(VERSION)

deb-clean:
	$(MAKE) -C .. deb-clean VERSION=$(VERSION)

verify-deb-oss:
	$(MAKE) -C .. verify-deb-oss VERSION=$(VERSION)

dist-oss: package-oss
docker-build-oss:
	bash -c 'printf "%s\n" "$(SOURCE_REVISION)" | grep -Eq "^[0-9a-fA-F]{40}(-dirty)?$$"'
	docker build -f $(DOCKERFILE) \
		--build-arg BUILD_EDITION=oss \
		--build-arg VERSION=$(VERSION) \
		--build-arg SOURCE_REVISION=$(SOURCE_REVISION) \
		-t acugis/report-server:$(VERSION)-oss \
		-t acugis/report-server:oss \
		$(DOCKER_CONTEXT)

docker-push-oss:
	docker push acugis/report-server:$(VERSION)-oss
	docker push acugis/report-server:oss

