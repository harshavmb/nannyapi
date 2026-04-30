#!/bin/bash
set -e

echo "Cleaning pb_data..."
rm -rf pb_data

echo "Killing nannyapi processes if any"
pkill -f nannyapi || true

echo "Building..."
go build

echo "Loading environment variables..."
if [ -f .env ]; then
  export $(cat .env | grep -v '^#' | xargs)
  echo "Loaded environment variables from .env"
else
  echo "No .env file found"
fi

echo "Running migrations with OAuth credentials..."
./nannyapi migrate --dir=./pb_data

echo "Creating default admin..."
./nannyapi superuser upsert admin@nannyapi.local AdminPass-123 --dir=./pb_data

echo "Setup complete. Admin: admin@nannyapi.local / AdminPass-123"
echo "Starting server with NANNYAPI_PRICING_CONFIG..."
NANNYAPI_PRICING_CONFIG="./pricing.config.json" ./nannyapi serve --dir=./pb_data --http="0.0.0.0:8090"
