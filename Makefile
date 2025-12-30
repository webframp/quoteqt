.PHONY: build clean test test-unit test-integration run restart stop

# Build variables
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT_SHA ?= $(shell git rev-parse HEAD 2>/dev/null || echo "unknown")
LDFLAGS := -X github.com/webframp/quoteqt/srv.Version=$(VERSION) -X github.com/webframp/quoteqt/srv.CommitSHA=$(COMMIT_SHA)

build:
	go build -ldflags "$(LDFLAGS)" -o bin/srv ./cmd/srv

clean:
	rm -f bin/srv

# Run the server (foreground)
run: build
	./bin/srv

# Restart the server via systemd (rebuild and restart)
restart: build
	sudo systemctl restart quotes

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

# k6 load testing
# Install: https://k6.io/docs/getting-started/installation/

# Quick k6 test - 10 VUs for 10s
k6-quick:
	k6 run k6/quick.js

# Realistic Nightbot simulation
k6-realistic:
	k6 run k6/realistic.js

# Full scenario test (normal, burst, nightbot)
k6-scenarios:
	k6 run k6/scenarios.js

# k6 multi-channel Nightbot simulation
k6-nightbot:
	k6 run k6/nightbot-channels.js
