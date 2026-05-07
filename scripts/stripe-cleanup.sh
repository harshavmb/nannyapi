#!/usr/bin/env bash
# stripe-cleanup.sh – Removes test resources from the Stripe sandbox.
#
# Deletes: subscriptions, customers, and archives products.
# Safe to run multiple times.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

# Load .env
if [[ -f "$PROJECT_ROOT/.env" ]]; then
    # shellcheck disable=SC1091
    source "$PROJECT_ROOT/.env"
fi

if [[ -z "${STRIPE_SECRET_KEY:-}" ]]; then
    echo "ERROR: STRIPE_SECRET_KEY not set."
    exit 1
fi

STRIPE_API="https://api.stripe.com/v1"
AUTH="$STRIPE_SECRET_KEY:"

echo "=== Stripe Sandbox Cleanup ==="

# Cancel active subscriptions
echo ""
echo "Canceling subscriptions..."
SUBS=$(curl -s -u "$AUTH" "$STRIPE_API/subscriptions?limit=100")
echo "$SUBS" | python3 -c "
import sys, json
data = json.load(sys.stdin).get('data', [])
for s in data:
    print(s['id'])
" 2>/dev/null | while read -r sub_id; do
    curl -s -u "$AUTH" -X DELETE "$STRIPE_API/subscriptions/$sub_id" > /dev/null
    echo "  Canceled: $sub_id"
done

# Delete customers
echo ""
echo "Deleting customers..."
CUSTOMERS=$(curl -s -u "$AUTH" "$STRIPE_API/customers?limit=100")
echo "$CUSTOMERS" | python3 -c "
import sys, json
data = json.load(sys.stdin).get('data', [])
for c in data:
    print(c['id'], c.get('email', ''))
" 2>/dev/null | while read -r cust_id email; do
    curl -s -u "$AUTH" -X DELETE "$STRIPE_API/customers/$cust_id" > /dev/null
    echo "  Deleted: $cust_id ($email)"
done

# Archive products
echo ""
echo "Archiving products..."
PRODUCTS=$(curl -s -u "$AUTH" "$STRIPE_API/products?limit=100&active=true")
echo "$PRODUCTS" | python3 -c "
import sys, json
data = json.load(sys.stdin).get('data', [])
for p in data:
    print(p['id'], p.get('name', ''))
" 2>/dev/null | while read -r prod_id name; do
    curl -s -u "$AUTH" "$STRIPE_API/products/$prod_id" -d "active=false" > /dev/null
    echo "  Archived: $prod_id ($name)"
done

echo ""
echo "=== Cleanup complete ==="
