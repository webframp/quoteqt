#!/bin/bash
set -e

echo "==> Running unit tests"
make test-unit

echo "==> Building"
make build

echo "==> Restarting service"
sudo systemctl restart quotes

echo "==> Waiting for service to start"
sleep 2

echo "==> Running integration tests"
make test-integration

echo "==> Deploy complete!"
