#!/usr/bin/env sh
# migrate.sh — Run database migrations before deploy.
# In a real project this would run something like:
#   goose -dir ./migrations postgres "$DATABASE_URL" up
# For the tutorial, we simply log the action.

set -e

echo "📦 Running database migrations..."
echo "   (no-op for tutorial — add real migration logic here)"
echo "✅ Migrations complete."
