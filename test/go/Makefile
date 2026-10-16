.PHONY: test vet fmt lint vuln clean

# This module (github.com/boru-lang/boru/test/go) is the shared TSV
# spec-runner scaffolding used by eng/go and lang's spec tests.

test:
	go test ./...

vet:
	go vet -copylocks=false ./...

fmt:
	gofmt -w .

# golangci-lint (config: ../../.golangci.yml, found by walking up).
lint:
	golangci-lint run ./...

vuln:
	govulncheck ./...

clean:
	go clean -testcache
