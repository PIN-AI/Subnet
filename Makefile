SHELL := /bin/bash

.PHONY: all build test proto clean

all: build

build: build-matcher build-simple-agent build-mock-rootlayer build-validator build-test-agent build-scripts

# Auto-detect architecture (default to amd64 for compatibility)
ARCH ?= $(shell uname -m | sed 's/x86_64/amd64/g' | sed 's/aarch64/arm64/g')
ifeq ($(ARCH),)
	ARCH = amd64
endif

# Output directory for Linux binaries (organized by architecture)
LINUX_BIN_DIR = bin/linux-$(ARCH)

# Build Linux binaries for Docker deployment (auto-detect architecture)
build-linux: build-linux-matcher build-linux-validator build-linux-simple-agent
	@echo "✓ Built Linux binaries for $(ARCH) in $(LINUX_BIN_DIR)/"

# Build Linux binaries for AMD64 (x86_64) - most common server architecture
build-linux-amd64:
	@echo "Building for Linux AMD64..."
	@ARCH=amd64 $(MAKE) build-linux

# Build Linux binaries for ARM64 (Apple Silicon, ARM servers)
build-linux-arm64:
	@echo "Building for Linux ARM64..."
	@ARCH=arm64 $(MAKE) build-linux

build-linux-matcher:
	@echo "Building Matcher for Linux $(ARCH)..."
	@mkdir -p $(LINUX_BIN_DIR)
	@CGO_ENABLED=0 GOOS=linux GOARCH=$(ARCH) go build -o $(LINUX_BIN_DIR)/matcher cmd/matcher/main.go

# build-linux-registry: REMOVED - Registry component deprecated

build-linux-validator:
	@echo "Building Validator for Linux $(ARCH)..."
	@mkdir -p $(LINUX_BIN_DIR)
	@CGO_ENABLED=0 GOOS=linux GOARCH=$(ARCH) go build -o $(LINUX_BIN_DIR)/validator cmd/validator/main.go

build-linux-simple-agent:
	@echo "Building Simple Agent for Linux $(ARCH)..."
	@mkdir -p $(LINUX_BIN_DIR)
	@CGO_ENABLED=0 GOOS=linux GOARCH=$(ARCH) go build -o $(LINUX_BIN_DIR)/simple-agent cmd/simple-agent/main.go

build-all:
	@echo "Building all Go packages..."
	@go build ./...

build-matcher:
	@echo "Building Matcher..."
	@go build -o bin/matcher cmd/matcher/main.go

build-simple-agent:
	@echo "Building Simple Agent..."
	@go build -o bin/simple-agent cmd/simple-agent/main.go

build-mock-rootlayer:
	@echo "Building Mock RootLayer..."
	@go build -o bin/mock-rootlayer cmd/mock-rootlayer/main.go

build-validator:
	@echo "Building Validator..."
	@go build -o bin/validator cmd/validator/main.go

# build-registry: REMOVED - Registry component deprecated

build-test-agent:
	@echo "Building Test Agent..."
	@go build -o bin/test-agent scripts/test-agent/validator_test_agent.go

# Build utility scripts
build-scripts: build-register-participants build-submit-intent-signed build-derive-pubkey build-create-subnet

build-register-participants:
	@echo "Building Register Participants..."
	@go build -o bin/register-participants scripts/register-participants.go

build-submit-intent-signed:
	@echo "Building Submit Intent Signed..."
	@go build -o bin/submit-intent-signed scripts/submit-intent-signed.go

build-derive-pubkey:
	@echo "Building Derive Pubkey..."
	@go build -o bin/derive-pubkey scripts/derive-pubkey.go

build-create-subnet:
	@echo "Building Create Subnet..."
	@go build -o bin/create-subnet scripts/create-subnet.go

test:
	@echo "Running tests..."
	@go test ./...

# Protobuf codegen
# NOTE: Proto files are already generated and included in the repository.
# The 'make proto' target is only needed for developers who have access to
# the pin_protocol source repository and want to regenerate the proto files.
# GOOGLEAPIS path for google/api/annotations.proto
GOOGLEAPIS := $(shell find $(HOME)/go/pkg/mod -path "*/grpc-gateway*/third_party/googleapis" -type d 2>/dev/null | head -1)

proto: proto-from-pin proto-rootlayer proto-common

# Generate from canonical definitions under pin_protocol/proto (authoritative)
# NOTE: Requires ../pin_protocol directory with proto source files
.PHONY: proto-from-pin proto-rootlayer
proto-from-pin:
	@echo "[INFO] Generating Subnet protobuf code from ../pin_protocol/proto/subnet into ./proto/subnet ..."
	@if [ ! -d "../pin_protocol/proto" ]; then \
		echo "[WARN] ../pin_protocol/proto not found. Proto files are already generated in ./proto/"; \
		echo "[WARN] This target is only for developers with access to pin_protocol source."; \
		exit 0; \
	fi
	@mkdir -p ./proto/subnet
	@find ./proto/subnet -type f -name '*.pb.go' -delete
	@protoc -I ../pin_protocol \
		--go_out=paths=source_relative:. \
		--go-grpc_out=paths=source_relative:. \
		../pin_protocol/proto/subnet/*.proto
	@echo "[INFO] Removing deprecated registry service proto files..."
	@rm -f ./proto/subnet/registry_service.pb.go ./proto/subnet/registry_service_grpc.pb.go
	@echo "[INFO] Protobuf code generation from pin_protocol (subnet) complete"

proto-rootlayer:
	@echo "[INFO] Generating RootLayer protobuf code into ./proto/rootlayer ..."
	@if [ ! -d "../pin_protocol/proto" ]; then \
		echo "[WARN] ../pin_protocol/proto not found. Proto files are already generated in ./proto/"; \
		exit 0; \
	fi
	@if [ -z "$(GOOGLEAPIS)" ]; then \
		echo "ERROR: googleapis not found. Install: go get github.com/grpc-ecosystem/grpc-gateway"; \
		exit 1; \
	fi
	@echo "[INFO] Using googleapis from: $(GOOGLEAPIS)"
	@mkdir -p ./proto/rootlayer
	@find ./proto/rootlayer -type f -name '*.pb.go' -delete
	@find ./proto/rootlayer -type f -name 'compat.go' -delete
	@find ./proto/rootlayer -type f -name 'service_types.go' -delete
	@rm -rf ./rootlayer
	@protoc -I ../pin_protocol/proto \
		-I $(GOOGLEAPIS) \
		--go_out=. \
		--go-grpc_out=. \
		--go_opt=Mrootlayer/intent.proto=subnet/proto/rootlayer \
		--go_opt=Mrootlayer/assignment.proto=subnet/proto/rootlayer \
		--go_opt=Mrootlayer/validation.proto=subnet/proto/rootlayer \
		--go_opt=Mrootlayer/service.proto=subnet/proto/rootlayer \
		--go_opt=Mrootlayer/error_reason.proto=subnet/proto/rootlayer \
		--go-grpc_opt=Mrootlayer/intent.proto=subnet/proto/rootlayer \
		--go-grpc_opt=Mrootlayer/assignment.proto=subnet/proto/rootlayer \
		--go-grpc_opt=Mrootlayer/validation.proto=subnet/proto/rootlayer \
		--go-grpc_opt=Mrootlayer/service.proto=subnet/proto/rootlayer \
		--go-grpc_opt=Mrootlayer/error_reason.proto=subnet/proto/rootlayer \
		../pin_protocol/proto/rootlayer/*.proto
	@if [ -d ./subnet/proto/rootlayer ]; then \
		cp ./subnet/proto/rootlayer/*.pb.go ./proto/rootlayer/ 2>/dev/null || true; \
		rm -rf ./subnet; \
	fi
	@rm -rf ./rootlayer
	@echo "[INFO] Protobuf code generation from pin_protocol (rootlayer) complete"

proto-common:
	@echo "[INFO] Generating common protobuf code into ./proto/common ..."
	@if [ ! -d "../pin_protocol/proto" ]; then \
		echo "[WARN] ../pin_protocol/proto not found. Proto files are already generated in ./proto/"; \
		exit 0; \
	fi
	@mkdir -p ./proto/common
	@find ./proto/common -type f -name '*.pb.go' -delete
	@protoc -I ../pin_protocol \
		--go_out=paths=source_relative:. \
		../pin_protocol/proto/common/*.proto
	@echo "[INFO] Protobuf code generation from pin_protocol (common) complete"

clean:
	@echo "Cleaning build artifacts..."
	@rm -rf bin dist validator agent
	@rm -f proto/*.pb.go
	@rm -rf proto/subnet proto/rootlayer proto/common
