#!/bin/make

GOROOT:=$(shell PATH="/pkg/main/dev-lang.go.dev/bin:$$PATH" go env GOROOT)
GO_TAG:=$(shell /bin/sh -c 'eval `$(GOROOT)/bin/go tool dist env`; echo "$${GOOS}_$${GOARCH}"')
GIT_TAG:=$(shell git rev-parse --short HEAD)
CHANNEL:=$(shell git rev-parse --abbrev-ref HEAD)
GOPATH:=$(shell $(GOROOT)/bin/go env GOPATH)
SOURCES:=$(shell find . -name '*.go')
AWS:=$(shell PATH="$$HOME/.local/bin:$$PATH" which 2>/dev/null aws)
S3_TARGET=s3://dist-go
DEFAULT_CHANNEL?=master
API_PREFIX?=https://ws.atonline.com/_special/rest/
CT_HEADER?=Content-Type: application/zip
ifeq ($(DATE_TAG),)
DATE_TAG:=$(shell TZ=UTC git show -s --format=%cd --date=format-local:%Y%m%d%H%M%S HEAD)
endif
export DATE_TAG
export GO111MODULE=on

# do we have a defined target arch?
ifneq ($(TARGET_ARCH),)
TARGET_ARCH_SPACE:=$(subst _, ,$(TARGET_ARCH))
TARGET_GOOS=$(word 1,$(TARGET_ARCH_SPACE))
TARGET_GOARCH=$(word 2,$(TARGET_ARCH_SPACE))
endif

-include contrib/config.mak

# variables that should be set in contrib/config.mak
ifeq ($(DIST_ARCHS),)
DIST_ARCHS=linux_amd64 linux_386 linux_arm linux_arm64 linux_ppc64 linux_ppc64le darwin_amd64 darwin_386 freebsd_386 freebsd_amd64 freebsd_arm windows_386 windows_amd64
endif
ifeq ($(PROJECT_NAME),)
PROJECT_NAME:=$(shell basename `pwd`)
endif

.PHONY: all deps update fmt test check doc dist update-make gen cov

all: $(PROJECT_NAME)

$(PROJECT_NAME): $(SOURCES)
	GOROOT="$(GOROOT)" $(GOPATH)/bin/goimports -w -l .
	$(GOROOT)/bin/go build -v -tags "$(GO_TAGS)" -gcflags="-N -l" -ldflags=all="-X github.com/KarpelesLab/goupd.MODE=DEV -X github.com/KarpelesLab/goupd.CHANNEL=$(CHANNEL) $(GOLDFLAGS)"

clean:
	$(GOROOT)/bin/go clean

deps:
	$(GOROOT)/bin/go get -v .

update:
	$(GOROOT)/bin/go get -u .

fmt:
	$(GOROOT)/bin/go fmt ./...
	$(GOPATH)/bin/goimports -w -l .

test:
	$(GOROOT)/bin/go test -v ./...

gen:
	$(GOROOT)/bin/go generate

cov:
	$(GOROOT)/bin/go test -coverprofile=coverage.out ./...
	$(GOROOT)/bin/go tool cover -html=coverage.out -o coverage.html

check:
	@if [ ! -f $(GOPATH)/bin/gometalinter ]; then $(GOROOT)/bin/go get github.com/alecthomas/gometalinter; fi
	$(GOPATH)/bin/gometalinter ./...

doc:
	@if [ ! -f $(GOPATH)/bin/godoc ]; then $(GOROOT)/bin/go get golang.org/x/tools/cmd/godoc; fi
	$(GOPATH)/bin/godoc -v -http=:6060 -index -play

dist:
	@mkdir -p dist/$(PROJECT_NAME)_$(CHANNEL)_$(GIT_TAG)/upload
	@make -s $(patsubst %,dist/$(PROJECT_NAME)_$(CHANNEL)_$(GIT_TAG)/upload/$(PROJECT_NAME)_%.bz2,$(DIST_ARCHS))
ifneq ($(AWS),)
	@echo "Uploading ..."
	@$(AWS) s3 cp --cache-control 'max-age=31536000' --recursive "dist/$(PROJECT_NAME)_$(CHANNEL)_$(GIT_TAG)/upload" "$(S3_TARGET)/$(PROJECT_NAME)/$(PROJECT_NAME)_$(CHANNEL)_$(DATE_TAG)_$(GIT_TAG)/"
	@echo "Configuring dist repository"
	@echo "$(DIST_ARCHS)" | $(AWS) s3 cp --cache-control 'max-age=31536000' --content-type 'text/plain' - "$(S3_TARGET)/$(PROJECT_NAME)/$(PROJECT_NAME)_$(CHANNEL)_$(DATE_TAG)_$(GIT_TAG).arch"
	@echo "$(DATE_TAG) $(GIT_TAG) $(PROJECT_NAME)_$(CHANNEL)_$(DATE_TAG)_$(GIT_TAG)" | $(AWS) s3 cp --cache-control 'max-age=60' --content-type 'text/plain' - "$(S3_TARGET)/$(PROJECT_NAME)/LATEST-$(CHANNEL)"
ifeq ($(CHANNEL),$(DEFAULT_CHANNEL))
	@echo "$(DATE_TAG) $(GIT_TAG) $(PROJECT_NAME)_$(CHANNEL)_$(DATE_TAG)_$(GIT_TAG)" | $(AWS) s3 cp --cache-control 'max-age=60' --content-type 'text/plain' - "$(S3_TARGET)/$(PROJECT_NAME)/LATEST"
endif
	@echo "Sending to production complete!"
endif
ifneq ($(GO_RELEASE_KEY),)
	zip -0r dist/$(PROJECT_NAME)_$(CHANNEL)_$(GIT_TAG).zip dist/$(PROJECT_NAME)_$(CHANNEL)_$(GIT_TAG)/upload
	curl >dist/upload_info.json -s --data-urlencode "key=$(GO_RELEASE_KEY)" --data-urlencode "package=$(PROJECT_NAME)" --data-urlencode "channel=$(CHANNEL)" --data-urlencode "tag=$(PROJECT_NAME)_$(CHANNEL)_$(DATE_TAG)_$(GIT_TAG)" "${API_PREFIX}Cloud/Go:release"
	sha256sum -b dist/$(PROJECT_NAME)_$(CHANNEL)_$(GIT_TAG).zip
	curl -T "dist/$(PROJECT_NAME)_$(CHANNEL)_$(GIT_TAG).zip" -H "$(CT_HEADER)" "$$(cat dist/upload_info.json | jq -r .data.PUT -)" >/dev/null
	curl -s "$(API_PREFIX)$$(cat dist/upload_info.json | jq -r .data.Complete -)"; echo
endif
ifneq ($(NOTIFY),)
	@echo "Sending notify..."
	@curl -s "$(NOTIFY)"
endif

dist/$(PROJECT_NAME)_$(CHANNEL)_$(GIT_TAG)/upload/$(PROJECT_NAME)_%.bz2: dist/$(PROJECT_NAME)_$(CHANNEL)_$(GIT_TAG)/$(PROJECT_NAME).%
	@echo "Generating $@"
	@bzip2 --stdout --compress --keep -9 "$<" >"$@"

dist/$(PROJECT_NAME)_$(CHANNEL)_$(GIT_TAG):
	@mkdir "$@"

dist/$(PROJECT_NAME)_$(CHANNEL)_$(GIT_TAG)/$(PROJECT_NAME).%: $(SOURCES)
	@echo " * Building $(PROJECT_NAME)[$(CHANNEL)] for $*"
	@TARGET_ARCH="$*" make -s dist/$(PROJECT_NAME)_$(CHANNEL)_$(GIT_TAG)/build_$(PROJECT_NAME).$*
	@mv 'dist/$(PROJECT_NAME)_$(CHANNEL)_$(GIT_TAG)/build_$(PROJECT_NAME).$*' 'dist/$(PROJECT_NAME)_$(CHANNEL)_$(GIT_TAG)/$(PROJECT_NAME).$*'

ifneq ($(TARGET_ARCH),)
dist/$(PROJECT_NAME)_$(CHANNEL)_$(GIT_TAG)/build_$(PROJECT_NAME).$(TARGET_ARCH): $(SOURCES)
	@GOOS="$(TARGET_GOOS)" GOARCH="$(TARGET_GOARCH)" $(GOROOT)/bin/go build -a -o "$@" -tags "$(GO_TAGS)" -gcflags="-N -l -trimpath=$(shell pwd)" -ldflags=all="-s -w -X github.com/KarpelesLab/goupd.MODE=PROD -X github.com/KarpelesLab/goupd.CHANNEL=$(CHANNEL) $(GOLDFLAGS)"
endif

update-make:
	@echo "Updating Makefile ..."
	@curl -s "https://raw.githubusercontent.com/KarpelesLab/make-go/master/Makefile" >Makefile.upd
	@mv -f "Makefile.upd" "Makefile"
