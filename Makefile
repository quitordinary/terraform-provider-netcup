# Every check runs without credentials and without network access to netcup.
TOOL := go tool -modfile=tools/go.mod

.PHONY: check test lint vuln build tidy-check

check: tidy-check test lint vuln

test:
	go vet ./...
	go test -race -count=1 ./...

lint:
	$(TOOL) golangci-lint run ./...

vuln:
	$(TOOL) govulncheck ./...

build:
	CGO_ENABLED=0 go build -trimpath -o bin/netcupctl ./cmd/netcupctl

# go.mod and go.sum must already be tidy; CI never rewrites them.
tidy-check:
	go mod tidy -diff
	cd tools && go mod tidy -diff
