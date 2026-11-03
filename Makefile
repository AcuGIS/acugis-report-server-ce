# AcuGIS Report Server — native package and Docker release targets.
# Application compile targets live in go-report-server/Makefile.

.PHONY: package package-oss \
	verify-package verify-package-oss \
	deb deb-oss deb-clean \
	verify-deb verify-deb-oss \
	test-deb-install \
	docker-build-oss \
	docker-push-oss \
	build-oss

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo 0.0.0)
# Host-side commit id. An export with no .git uses its SOURCE_REVISION file.
# The image build context does not include .git; pass the value explicitly.
SOURCE_REVISION ?= $(shell bash -c 'source scripts/source-revision.sh && resolve_source_revision "$$(pwd)"')
DEB_ARCH ?= amd64
DEB_REVISION ?=
DOCKERFILE ?= docker/report-server/Dockerfile
DOCKER_CONTEXT ?= .

# Public Docker Hub repository. These names are literals.
# An unset DOCKER_REGISTRY must not resolve to docker.io/library/.

package-oss:
	bash scripts/package-release.sh oss $(VERSION)

package: package-oss

verify-package-oss:
	bash scripts/verify-package.sh oss $(VERSION)

verify-package: verify-package-oss

# Debian packages. On Linux amd64, scripts/package-deb.sh compiles with host
# Go from the current tree (no Docker, no tarball reuse). Docker remains
# optional for Windows/cross-platform (PACKAGE_USE_DOCKER=1).
deb-oss:
	DEB_ARCH=$(DEB_ARCH) DEB_REVISION=$(DEB_REVISION) bash scripts/package-deb.sh oss $(VERSION)

deb: deb-oss

# Remove staging/output artifacts, then build both DEBs from the current tree.
deb-clean:
	rm -rf dist/stage-oss dist/stage-professional \
		dist/deb-tree-oss dist/deb-tree-professional
	rm -rf dist/deb-extras-* dist/deb-extract-*
	rm -f dist/*.deb dist/*.deb.sha256 \
		dist/acugis-report-server-oss-*-linux-amd64.tar.gz \
		dist/acugis-report-server-oss-*-linux-amd64.tar.gz.sha256 \
		dist/acugis-report-server-professional-*-linux-amd64.tar.gz \
		dist/acugis-report-server-professional-*-linux-amd64.tar.gz.sha256
	$(MAKE) deb VERSION=$(VERSION)

verify-deb-oss:
	DEB_ARCH=$(DEB_ARCH) DEB_REVISION=$(DEB_REVISION) bash scripts/verify-deb.sh oss $(VERSION)

verify-deb: verify-deb-oss

test-deb-install:
	DEB_ARCH=$(DEB_ARCH) bash scripts/test-deb-install.sh $(VERSION)

build-oss:
	$(MAKE) -C go-report-server build-oss VERSION=$(VERSION)

docker-build-oss:
	bash -c 'printf "%s\n" "$(SOURCE_REVISION)" | grep -Eq "^[0-9a-fA-F]{40}(-dirty)?$$"'
	docker build -f $(DOCKERFILE) \
		--build-arg BUILD_EDITION=oss \
		--build-arg VERSION=$(VERSION) \
		--build-arg SOURCE_REVISION=$(SOURCE_REVISION) \
		-t acugis/report-server:$(VERSION)-oss \
		-t acugis/report-server:oss \
		$(DOCKER_CONTEXT)

# Explicit push only — never invoked by package targets.
docker-push-oss:
	docker push acugis/report-server:$(VERSION)-oss
	docker push acugis/report-server:oss

