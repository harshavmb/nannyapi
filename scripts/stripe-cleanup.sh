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
echo ""
echo "WARNING: This will cancel subscriptions, delete customers, and archive"
echo "products created by nannyapi in the connected Stripe account."
echo ""
read -rp "Are you sure you want to proceed? (yes/no): " CONFIRM
if [[ "$CONFIRM" != "yes" ]]; then
    echo "Aborted."
    exit 0
fi

# Cancel active subscriptions (scoped to nannyapi customers)
echo ""
echo "Canceling subscriptions..."
SUBS=$(curl -s -u "$AUTH" "$STRIPE_API/subscriptions?limit=100")
echo "$SUBS" | python3 -c "
import sys, json
data = json.load(sys.stdin).get('data', [])
for s in data:
    meta = s.get('metadata', {})
    if meta.get('source') == 'nannyapi' or not meta:
        print(s['id'])
" 2>/dev/null | while read -r sub_id; do
    curl -s -u "$AUTH" -X DELETE "$STRIPE_API/subscriptions/$sub_id" > /dev/null
    echo "  Canceled: $sub_id"
done

# Delete customers (scoped to nannyapi-created customers by metadata)
echo ""
echo "Deleting customers..."
CUSTOMERS=$(curl -s -u "$AUTH" "$STRIPE_API/customers?limit=100")
echo "$CUSTOMERS" | python3 -c "
import sys, json
data = json.load(sys.stdin).get('data', [])
for c in data:
    meta = c.get('metadata', {})
    if meta.get('source') == 'nannyapi' or not meta:
        print(c['id'], c.get('email', ''))
" 2>/dev/null | while read -r cust_id email; do
    curl -s -u "$AUTH" -X DELETE "$STRIPE_API/customers/$cust_id" > /dev/null
    echo "  Deleted: $cust_id ($email)"
done

# Archive products (only those with nannyapi in name or metadata)
echo ""
echo "Archiving products..."
PRODUCTS=$(curl -s -u "$AUTH" "$STRIPE_API/products?limit=100&active=true")
echo "$PRODUCTS" | python3 -c "
import sys, json
data = json.load(sys.stdin).get('data', [])
for p in data:
    name = p.get('name', '')
    meta = p.get('metadata', {})
    if 'nannyapi' in name.lower() or 'nanny' in name.lower() or meta.get('source') == 'nannyapi' or not meta:
        print(p['id'], name)
" 2>/dev/null | while read -r prod_id name; do
    curl -s -u "$AUTH" "$STRIPE_API/products/$prod_id" -d "active=false" > /dev/null
    echo "  Archived: $prod_id ($name)"
done

echo ""
echo "=== Cleanup complete ==="
