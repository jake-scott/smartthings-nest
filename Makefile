SWAG = api/swagger.yml

current_dir := $(dir $(abspath $(lastword $(MAKEFILE_LIST))))

GOPATH          ?= $(shell go env GOPATH)

## Find the 'highest' vM.m.p version tag if there is one, else
#  use the commit ID
GITTAGS     := $(shell git tag --points-at HEAD | sort -V)
GITVER          := $(patsubst v%,%,$(filter v%, $(GITTAGS)))
MAXVER          := $(lastword $(GITVER))
ISDIRTY         := $(shell git diff-index --quiet HEAD -- || echo yes)

ifndef MAXVER
 MAXVER = commit-$(shell git rev-parse --short HEAD)
endif

## But always use 'dev' if there are changes in the index
ifdef ISDIRTY
  MAXVER = dev
endif


GOBIN ?= $(shell go env GOBIN)

ifeq (${GOBIN},)
	GOBIN = $(shell go env GOPATH)/bin
endif


.DEFAULT: build

.PHONY: build
build: fmtcheck
	go install -ldflags '-X github.com/jake-scott/smartthings-nest/version.Version=$(MAXVER)'

.PHONY: fmtcheck
fmtcheck:
	@"$(CURDIR)/scripts/gofmtcheck.sh"

.PHONY: lint
lint:
	golangci-lint run ./...

.PHONY: tools
tools:  $(GOBIN)/swagger
	@echo "==> installing required tooling..."
	GO111MODULE=off go get -u github.com/client9/misspell/cmd/misspell
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(GOPATH)/bin v1.31.0


$(GOBIN)/swagger:
	go install github.com/go-swagger/go-swagger/cmd/swagger

.PHONY: models
models: generated
	go generate ./...

generated:
	mkdir -p $@