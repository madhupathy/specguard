#!/bin/bash

# Database migration runner for SpecGuard

set -e

COMMAND=${1:-"up"}

echo "🗄️  Running database migrations..."

# Check if database is accessible
if ! PGPASSWORD=$DB_PASSWORD psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME -c "SELECT 1;" &> /dev/null; then
    echo "❌ Cannot connect to database. Please check your configuration:"
    echo "   DB_HOST: $DB_HOST"
    echo "   DB_PORT: $DB_PORT" 
    echo "   DB_USER: $DB_USER"
    echo "   DB_NAME: $DB_NAME"
    exit 1
fi

echo "✅ Database connection successful"

case $COMMAND in
    "up")
        echo "📈 Running migrations..."
        go run cmd/migrate/main.go up
        echo "✅ Migrations completed"
        ;;
    "down")
        echo "📉 Rolling back last migration..."
        go run cmd/migrate/main.go down
        echo "✅ Rollback completed"
        ;;
    "create")
        MIGRATION_NAME=${2:-""}
        if [ -z "$MIGRATION_NAME" ]; then
            echo "❌ Migration name required: ./scripts/migrate.sh create migration_name"
            exit 1
        fi
        echo "📝 Creating migration: $MIGRATION_NAME"
        go run cmd/migrate/main.go create $MIGRATION_NAME
        echo "✅ Migration created"
        ;;
    "status")
        echo "📊 Migration status:"
        go run cmd/migrate/main.go status
        ;;
    *)
        echo "❌ Unknown command: $COMMAND"
        echo "Available commands:"
        echo "  up      - Run all pending migrations"
        echo "  down    - Rollback last migration"
        echo "  create  - Create new migration (requires name)"
        echo "  status  - Show migration status"
        exit 1
        ;;
esac
