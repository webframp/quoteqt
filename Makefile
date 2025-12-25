.PHONY: build clean test test-unit test-integration

build:
	go build -o srv ./cmd/srv

clean:
	rm -f srv

# Run unit tests only
test-unit:
	go test ./srv/...

# Run integration tests (requires server running on localhost:8000)
test-integration:
	go test -tags=integration -v ./...

# Run all tests
test: test-unit test-integration
