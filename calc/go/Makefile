.PHONY: build test test-v vet fmt lint cover cover-html clean run

BIN := bin/calc
COVER := coverage.out

build:
	@mkdir -p bin
	go build -o $(BIN) ./cmd/calc

test:
	go test ./...

test-v:
	go test -v ./...

vet:
	go vet -copylocks=false ./...

fmt:
	gofmt -w .

# golangci-lint uses the repo-root config (../.golangci.yml) when present.
lint:
	golangci-lint run ./...

# Coverage: write a coverage profile, print the per-package summary, and
# print the function-level top losers so the lowest-covered code is the
# obvious next thing to write tests for.
cover:
	go test -coverprofile=$(COVER) ./...
	@echo
	@echo "==> coverage by package:"
	@go tool cover -func=$(COVER) | tail -1
	@echo
	@echo "==> 10 least-covered functions:"
	@go tool cover -func=$(COVER) | grep -v '^total:' | sort -k3 -n | head -10

# Open the HTML coverage report in a browser. Useful for "which lines in
# words.go did the tests miss?"
cover-html: cover
	go tool cover -html=$(COVER)

# Run the CLI against a one-shot expression. Override EXPR=... to play.
EXPR ?= 10 sub 3
run: build
	./$(BIN) -e "$(EXPR)"

clean:
	rm -rf bin $(COVER)
	go clean -testcache
