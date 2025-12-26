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

# Load testing with hey
# Install: go install github.com/rakyll/hey@latest

# Quick smoke test - 100 requests, 5 concurrent
load-quick:
	@echo "=== Quick load test (100 requests) ==="
	~/go/bin/hey -n 100 -c 5 http://localhost:8000/api/quote

# Medium load test - 1000 requests, 10 concurrent
load-medium:
	@echo "=== Medium load test (1000 requests) ==="
	~/go/bin/hey -n 1000 -c 10 http://localhost:8000/api/quote

# Heavy load test - 5000 requests, 50 concurrent
load-heavy:
	@echo "=== Heavy load test (5000 requests) ==="
	~/go/bin/hey -n 5000 -c 50 http://localhost:8000/api/quote

# Test multiple endpoints with Nightbot headers
load-full:
	@echo "=== Full API load test ==="
	@echo "\n--- /api/quote ---"
	~/go/bin/hey -n 200 -c 10 http://localhost:8000/api/quote
	@echo "\n--- /api/quote?civ=hre ---"
	~/go/bin/hey -n 200 -c 10 "http://localhost:8000/api/quote?civ=hre"
	@echo "\n--- /api/matchup (with Nightbot header) ---"
	~/go/bin/hey -n 200 -c 10 -H "Nightbot-Channel: name=teststreamer&provider=twitch&providerId=12345" "http://localhost:8000/api/matchup?civ=hre&vs=french"
	@echo "\n--- /health ---"
	~/go/bin/hey -n 100 -c 10 http://localhost:8000/health
