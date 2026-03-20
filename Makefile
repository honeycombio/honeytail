MAKEFLAGS += --warn-undefined-variables
MAKEFLAGS += --no-builtin-rules
MAKEFLAGS += --no-builtin-variables

# Override with `make <some-target> VERSION=vX.Y.Z` or set VERSION in the environment
VERSION ?= $(shell git describe --tags --match='v[0-9]*' --always)

.PHONY: version
version:
	@echo $(VERSION)

.PHONY: build
build: GOOS ?= $(shell go env GOOS)
build: GOARCH ?= $(shell go env GOARCH)
build: ARTIFACTS ?= ./artifacts
build:
	GOOS=$(GOOS) GOARCH=$(GOARCH) \
    go build -ldflags "-X main.BuildID=$(VERSION)" -o $(ARTIFACTS)/honeytail-$(GOOS)-$(GOARCH) .
	@echo "Built: $(ARTIFACTS)/honeytail-$(GOOS)-$(GOARCH)"

.PHONY: test
test:
	go test --timeout 10s -v ./...

.PHONY: install-tools
install-tools:
	go install github.com/google/go-licenses/v2@v2.0.0-alpha.1

.PHONY: update-licenses
update-licenses: install-tools
	rm -rf LICENSES
	go-licenses save --save_path LICENSES .

.PHONY: verify-licenses
verify-licenses: install-tools
	go-licenses save --save_path temp .; \
    chmod +r temp; \
    if diff temp LICENSES; then \
      echo "Passed"; \
      rm -rf temp; \
    else \
      echo "LICENSES directory must be updated. Run make update-licenses"; \
      rm -rf temp; \
      exit 1; \
    fi; \
