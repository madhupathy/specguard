#!/bin/bash

# SpecGuard Development Setup Script

set -e

echo "🚀 Setting up SpecGuard development environment..."

# Check if Go is installed
if ! command -v go &> /dev/null; then
    echo "❌ Go is not installed. Please install Go 1.21 or later."
    exit 1
fi

# Check Go version
GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
REQUIRED_VERSION="1.21"

if [ "$(printf '%s\n' "$REQUIRED_VERSION" "$GO_VERSION" | sort -V | head -n1)" != "$REQUIRED_VERSION" ]; then
    echo "❌ Go version $GO_VERSION is too old. Please install Go 1.21 or later."
    exit 1
fi

echo "✅ Go version $GO_VERSION is compatible"

# Check if PostgreSQL is running
if ! command -v psql &> /dev/null; then
    echo "⚠️  PostgreSQL client not found. Please ensure PostgreSQL is installed and running."
    echo "   You can install PostgreSQL with: brew install postgresql (macOS) or apt-get install postgresql (Ubuntu)"
fi

# Create artifacts directory
mkdir -p artifacts
echo "✅ Created artifacts directory"

# Copy environment file if it doesn't exist
if [ ! -f .env ]; then
    cp .env.example .env
    echo "✅ Created .env file from .env.example"
    echo "   Please edit .env with your configuration"
else
    echo "✅ .env file already exists"
fi

# Install Go dependencies
echo "📦 Installing Go dependencies..."
go mod download
go mod verify
echo "✅ Dependencies installed"

# Run tests to verify setup
echo "🧪 Running tests..."
go test -v ./internal/... || {
    echo "❌ Tests failed. Please check the output above."
    exit 1
}
echo "✅ All tests passed"

# Build the application
echo "🔨 Building SpecGuard..."
go build -o bin/specguard cmd/specguard/main.go
echo "✅ Build completed successfully"

echo ""
echo "🎉 SpecGuard development environment is ready!"
echo ""
echo "Next steps:"
echo "1. Edit .env with your database and GitHub configuration"
echo "2. Start PostgreSQL if it's not running"
echo "3. Run database migrations: go run cmd/migrate/main.go up"
echo "4. Start the server: ./bin/specguard or go run cmd/specguard/main.go"
echo ""
echo "Server will be available at: http://localhost:8080"
echo "API documentation: http://localhost:8080/api/v1/health"
