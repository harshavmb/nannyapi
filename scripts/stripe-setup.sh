#!/usr/bin/env bash
# stripe-setup.sh – Creates required Stripe products and prices for nannyapi
# using the Stripe REST API directly (no Stripe CLI needed).
#
# Prerequisites:
#   - STRIPE_SECRET_KEY set in .env (sk_test_...)
#
# This script creates:
#   1. "nannyapi Pro" product with a €10/month recurring price
#   2. "nannyapi Credits" product with a €5 one-time price (1M tokens)
#
# It outputs the env vars to add to your .env file.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

# Load .env
if [[ -f "$PROJECT_ROOT/.env" ]]; then
    # shellcheck disable=SC1091
    source "$PROJECT_ROOT/.env"
fi

if [[ -z "${STRIPE_SECRET_KEY:-}" ]]; then
    echo "ERROR: STRIPE_SECRET_KEY not set. Add it to .env first."
    exit 1
fi

STRIPE_API="https://api.stripe.com/v1"
AUTH="$STRIPE_SECRET_KEY:"

echo "=== nannyapi Stripe Setup (REST API) ==="
echo ""

# --- Create Pro Plan Product ---
echo "Creating Pro Plan product..."
PRO_PRODUCT_RESP=$(curl -s -u "$AUTH" "$STRIPE_API/products" \
    -d "name=nannyapi Pro" \
    -d "description=Unlimited agents, 10M monthly tokens, unlimited investigations")

PRO_PRODUCT=$(echo "$PRO_PRODUCT_RESP" | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])" 2>/dev/null)
if [[ -z "$PRO_PRODUCT" || "$PRO_PRODUCT" == "null" ]]; then
    echo "ERROR creating product: $PRO_PRODUCT_RESP"
    exit 1
fi
echo "  Product: $PRO_PRODUCT"

# --- Create Pro Plan Price ($10/month) ---
echo "Creating Pro Plan price (€10/month)..."
PRO_PRICE_RESP=$(curl -s -u "$AUTH" "$STRIPE_API/prices" \
    -d "product=$PRO_PRODUCT" \
    -d "unit_amount=1000" \
    -d "currency=eur" \
    -d "recurring[interval]=month")

PRO_PRICE=$(echo "$PRO_PRICE_RESP" | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])" 2>/dev/null)
if [[ -z "$PRO_PRICE" || "$PRO_PRICE" == "null" ]]; then
    echo "ERROR creating price: $PRO_PRICE_RESP"
    exit 1
fi
echo "  Price: $PRO_PRICE"

# --- Create Credits Product ---
echo ""
echo "Creating Credits product..."
CREDITS_PRODUCT_RESP=$(curl -s -u "$AUTH" "$STRIPE_API/products" \
    -d "name=nannyapi Credits" \
    -d "description=1,000,000 extra tokens for the current billing period")

CREDITS_PRODUCT=$(echo "$CREDITS_PRODUCT_RESP" | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])" 2>/dev/null)
if [[ -z "$CREDITS_PRODUCT" || "$CREDITS_PRODUCT" == "null" ]]; then
    echo "ERROR creating product: $CREDITS_PRODUCT_RESP"
    exit 1
fi
echo "  Product: $CREDITS_PRODUCT"

# --- Create Credits Price ($5 one-time) ---
echo "Creating Credits price (€5/bundle, one-time)..."
CREDITS_PRICE_RESP=$(curl -s -u "$AUTH" "$STRIPE_API/prices" \
    -d "product=$CREDITS_PRODUCT" \
    -d "unit_amount=500" \
    -d "currency=eur")

CREDITS_PRICE=$(echo "$CREDITS_PRICE_RESP" | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])" 2>/dev/null)
if [[ -z "$CREDITS_PRICE" || "$CREDITS_PRICE" == "null" ]]; then
    echo "ERROR creating price: $CREDITS_PRICE_RESP"
    exit 1
fi
echo "  Price: $CREDITS_PRICE"

echo ""
echo "=== Done! Add these to your .env file ==="
echo ""
echo "STRIPE_PRO_PRICE_ID=$PRO_PRICE"
echo "STRIPE_CREDITS_PRICE_ID=$CREDITS_PRICE"
echo ""
echo "To forward webhooks locally, use:"
echo "  stripe listen --forward-to localhost:8090/api/stripe/webhook"
echo "  (or set STRIPE_SKIP_WEBHOOK_VERIFY=true for local dev)"
