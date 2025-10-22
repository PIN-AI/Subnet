SHELL := /bin/bash

.PHONY: all build test proto clean

all: build

build: build-matcher build-simple-agent build-mock-rootlayer build-registry build-validator build-test-agent build-scripts

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

build-registry:
	@echo "Building Registry Service..."
	@go build -o bin/registry cmd/registry/main.go

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
# GOOGLEAPIS path for google/api/annotations.proto
GOOGLEAPIS := $(shell find $(HOME)/go/pkg/mod -path "*/grpc-gateway*/third_party/googleapis" -type d 2>/dev/null | head -1)

proto: proto-from-pin proto-rootlayer proto-common

# Generate from canonical definitions under pin_protocol/proto (authoritative)
.PHONY: proto-from-pin proto-rootlayer
proto-from-pin:
	@echo "[INFO] Generating Subnet protobuf code from ../pin_protocol/proto/subnet into ./proto/subnet ..."
	@mkdir -p ./proto/subnet
	@find ./proto/subnet -type f -name '*.pb.go' -delete
	@protoc -I ../pin_protocol \
		--go_out=paths=source_relative:. \
		--go-grpc_out=paths=source_relative:. \
		../pin_protocol/proto/subnet/*.proto
	@echo "[INFO] Protobuf code generation from pin_protocol (subnet) complete"

proto-rootlayer:
	@echo "[INFO] Generating RootLayer protobuf code into ./proto/rootlayer ..."
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
