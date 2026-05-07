# Stripe Payment Integration

nannyapi supports optional billing via [Stripe](https://stripe.com). When the integration is not configured, all payment endpoints return a clear `503 Service Unavailable` response so self-hosted users experience a clean error rather than a crash.

---

## Table of Contents

1. [Quick Start](#quick-start)
2. [Environment Variables](#environment-variables)
3. [Stripe Dashboard Setup](#stripe-dashboard-setup)
4. [Pricing (Single Source of Truth)](#pricing-single-source-of-truth)
5. [API Endpoints](#api-endpoints)
6. [Subscription Lifecycle](#subscription-lifecycle)
7. [Webhooks & PocketBase Side Effects](#webhooks--pocketbase-side-effects)
8. [Buy Credits (Token Top-Up)](#buy-credits-token-top-up)
9. [Rate Limiting](#rate-limiting)
10. [Self-Hosted / No Stripe](#self-hosted--no-stripe)
11. [Testing](#testing)

---

## Quick Start

```bash
# 1. Copy your Stripe keys from https://dashboard.stripe.com/test/apikeys
export STRIPE_SECRET_KEY="sk_test_..."
export STRIPE_PUBLISHABLE_KEY="pk_test_..."

# 2. Run the setup script to create products and prices in Stripe:
bash scripts/stripe-setup.sh
# This outputs STRIPE_PRO_PRICE_ID and STRIPE_CREDITS_PRICE_ID — add them to .env

# 3. For local webhook testing (skip verification in dev):
export STRIPE_SKIP_WEBHOOK_VERIFY="true"

# 4. Run nannyapi
./nannyapi serve
```

---

## Environment Variables

| Variable | Required | Description |
|---|---|---|
| `STRIPE_SECRET_KEY` | Yes (to enable) | Stripe secret key (`sk_live_…` or `sk_test_…`). **Omit entirely to disable Stripe on self-hosted installs.** |
| `STRIPE_PUBLISHABLE_KEY` | No | Publishable key for client-side use. |
| `STRIPE_PRO_PRICE_ID` | Yes (if enabled) | Stripe Price ID for the Pro plan subscription (price from `pricing.config.json`). |
| `STRIPE_CREDITS_PRICE_ID` | Yes (for top-ups) | Stripe Price ID for a one-time credit bundle. |
| `STRIPE_WEBHOOK_SECRET` | Yes (production) | Webhook signing secret from your Stripe endpoint configuration. |
| `STRIPE_SKIP_WEBHOOK_VERIFY` | Dev only | Set to `"true"` to skip webhook signature verification. **Never use in production.** |

---

## Stripe Dashboard Setup

### Automated (Recommended)

Run the setup script which uses the Stripe REST API directly:

```bash
# Requires STRIPE_SECRET_KEY in .env
bash scripts/stripe-setup.sh
```

This creates products and prices matching `pricing.config.json` in EUR.

### Manual Setup

Create products in the Stripe Dashboard matching the values in `pricing.config.json`:

- **nannyapi Pro**: recurring price matching `tiers.pro.price_per_month` in `currency` (currently €10/month)
- **nannyapi Credits**: one-time price matching `credit_bundle_price` in `currency` (currently €5/bundle)

### Webhook Endpoint (Production)

1. Go to **Developers → Webhooks → Add endpoint**.
2. Endpoint URL: `https://your-domain.com/api/stripe/webhook`
3. Select these events:
   - `checkout.session.completed`
   - `customer.subscription.created`
   - `customer.subscription.updated`
   - `customer.subscription.deleted`
   - `invoice.payment_succeeded`
   - `invoice.payment_failed`
4. Copy the **Signing Secret** (`whsec_…`) → `STRIPE_WEBHOOK_SECRET`.

---

## Pricing (Single Source of Truth)

**All pricing is defined in `pricing.config.json`** — the Stripe prices must match these values:

```json
{
  "currency": "eur",
  "credit_bundle_tokens": 1000000,
  "credit_bundle_price": 5,
  "tiers": {
    "free": { "price_per_month": 0, "monthly_token_limit": 1000000, ... },
    "pro":  { "price_per_month": 10, "monthly_token_limit": 10000000, ... }
  }
}
```

| Plan | Price | Monthly Tokens | Agents | Investigations |
|------|-------|---------------|--------|----------------|
| **Free** | €0/mo | 1,000,000 | 2 max | 25/month |
| **Pro** | €10/mo | 10,000,000 | Unlimited | Unlimited |
| **Credit Bundle** | €5 one-time | +1,000,000 per bundle | — | — |

> **Important**: When changing prices, update `pricing.config.json` first, then create matching Stripe prices and update `STRIPE_PRO_PRICE_ID` / `STRIPE_CREDITS_PRICE_ID`. The config file is the source of truth for limits; Stripe is the source of truth for payment processing.

---

## API Endpoints

All endpoints require authentication (`Authorization: Bearer <token>`) except the webhook.

### `GET /api/stripe/subscription`

Returns the current user's subscription status.

**Response (200)** – Has subscription:
```json
{
  "has_subscription": true,
  "status": "active",
  "plan_name": "Pro",
  "current_period_end": "2025-02-01T00:00:00Z",
  "cancel_at_period_end": false
}
```

**Response (200)** – No subscription:
```json
{
  "has_subscription": false,
  "cancel_at_period_end": false
}
```

---

### `POST /api/stripe/subscribe`

Creates a Stripe Checkout Session for the Pro monthly subscription.

**Request**
```json
{
  "success_url": "https://app.example.com/settings?subscribed=true",
  "cancel_url": "https://app.example.com/settings"
}
```

**Response (200)**
```json
{
  "checkout_url": "https://checkout.stripe.com/..."
}
```

Redirect the user to `checkout_url`. Stripe handles card entry and confirmation.

**Errors**
- `503` – Stripe not configured
- `409` – User already has an active subscription

---

### `POST /api/stripe/cancel-subscription`

Schedules the active subscription to cancel at the end of the current billing period.

**Request**
```json
{
  "reason": "too expensive"
}
```

**Response (200)**
```json
{
  "message": "subscription will be cancelled at the end of the current billing period"
}
```

---

### `POST /api/stripe/reactivate-subscription`

Removes the pending cancellation so the subscription continues as normal.

**Response (200)**
```json
{
  "message": "subscription reactivated; billing will continue as normal"
}
```

---

### `POST /api/stripe/buy-credits`

Creates a one-time Stripe Checkout Session for extra token bundles (Pro subscribers only).

Each bundle adds tokens as configured in `pricing.config.json` → `credit_bundle_tokens` (default: 1,000,000).

**Request**
```json
{
  "quantity": 3,
  "success_url": "https://app.example.com/settings?credits=added",
  "cancel_url": "https://app.example.com/settings"
}
```

**Response (200)**
```json
{
  "checkout_url": "https://checkout.stripe.com/..."
}
```

**Errors**
- `503` – Stripe not configured
- `400` – No active Pro subscription / invalid quantity (must be 1–100)

---

### `GET /api/stripe/invoices`

Returns a paginated list of invoices for the authenticated user. On each call,
the server lazily syncs invoices from Stripe into a local `stripe_invoices`
collection so subsequent queries are fast.

**Auth:** Required (user token)

**Query Parameters:**

| Param      | Type | Default | Description                     |
|------------|------|---------|---------------------------------|
| `page`     | int  | 1       | Page number (1-indexed)         |
| `per_page` | int  | 10      | Items per page (max 100)        |

**Response (200):**

```json
{
  "items": [
    {
      "id": "record-id",
      "stripe_invoice_id": "in_1abc...",
      "invoice_number": "INV-0001",
      "status": "paid",
      "currency": "eur",
      "amount_due": 1500,
      "amount_paid": 1500,
      "period_start": "2026-04-01T00:00:00Z",
      "period_end": "2026-05-01T00:00:00Z",
      "invoice_created": "2026-04-01T00:00:00Z",
      "finalized_at": "2026-04-01T00:00:01Z",
      "paid_at": "2026-04-01T12:00:00Z",
      "description": "1 × Pro Plan (at €15.00 / month)",
      "pdf_download_url": "/api/stripe/invoices/record-id/pdf"
    }
  ],
  "page": 1,
  "per_page": 10,
  "total_items": 12,
  "total_pages": 2
}
```

**Notes for frontend:**
- Amounts are in the smallest currency unit (e.g. cents for EUR).
- `pdf_download_url` is a relative path; the frontend should call it with the
  auth token and follow the 307 redirect to download the PDF.
- Items are sorted newest-first by `invoice_created`.

---

### `GET /api/stripe/invoices/{id}/pdf`

Redirects (307) to the Stripe-hosted PDF URL for the given invoice.

**Auth:** Required (user token)

**Path Parameters:**

| Param | Description                                    |
|-------|------------------------------------------------|
| `id`  | The PocketBase record ID from the items list   |

**Response:**
- **307** – `Location` header contains the Stripe PDF URL. The PDF URL is
  short-lived and generated fresh on each request.
- **404** – Invoice not found or belongs to another user.

**Usage example (frontend):**

```javascript
const res = await fetch(`/api/stripe/invoices/${invoiceId}/pdf`, {
  headers: { Authorization: `Bearer ${token}` },
  redirect: 'manual'
});
const pdfUrl = res.headers.get('Location');
window.open(pdfUrl, '_blank');
```

---

### `POST /api/stripe/webhook`

Receives Stripe webhook events. Called by Stripe, not by the app frontend.

---

## Subscription Lifecycle

```
┌─────────────────────────────────────────────────────────────────┐
│                         SUBSCRIBE                                │
├─────────────────────────────────────────────────────────────────┤
│ 1. User calls POST /api/stripe/subscribe                        │
│ 2. Server creates Stripe Checkout Session                       │
│ 3. User redirected to Stripe Checkout → enters card → pays      │
│ 4. Stripe fires: checkout.session.completed                     │
│ 5. Webhook handler:                                             │
│    • Creates record in stripe_subscriptions (status=active)     │
│    • Sets users.tier = "pro"                                    │
│ 6. User now has Pro limits (per pricing.config.json)            │
└─────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────┐
│                          CANCEL                                  │
├─────────────────────────────────────────────────────────────────┤
│ 1. User calls POST /api/stripe/cancel-subscription              │
│ 2. Server sets cancel_at_period_end=true on Stripe              │
│ 3. Updates local stripe_subscriptions.cancel_at_period_end=true │
│ 4. User REMAINS Pro until current_period_end                    │
│ 5. At period end, Stripe fires: customer.subscription.deleted   │
│ 6. Webhook handler:                                             │
│    • Sets stripe_subscriptions.status = "canceled"              │
│    • Sets users.tier = "free"                                   │
└─────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────┐
│                       REACTIVATE                                 │
├─────────────────────────────────────────────────────────────────┤
│ 1. User calls POST /api/stripe/reactivate-subscription          │
│    (only valid while cancel_at_period_end=true)                 │
│ 2. Server sets cancel_at_period_end=false on Stripe             │
│ 3. Updates local record accordingly                             │
│ 4. Subscription continues as normal; no deletion event fired    │
└─────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────┐
│                       BUY CREDITS                                │
├─────────────────────────────────────────────────────────────────┤
│ 1. Pro user calls POST /api/stripe/buy-credits                  │
│ 2. Server creates one-time Stripe Checkout Session              │
│ 3. User completes payment                                       │
│ 4. Stripe fires: checkout.session.completed (mode=payment)      │
│ 5. Webhook handler:                                             │
│    • Retrieves line items to get quantity                        │
│    • Adds (quantity × credit_bundle_tokens) to                  │
│      user_limit_overrides.monthly_token_limit                   │
│ 6. User's effective monthly limit increases by purchased tokens │
└─────────────────────────────────────────────────────────────────┘
```

---

## Webhooks & PocketBase Side Effects

Webhook events are verified using the `Stripe-Signature` header and `STRIPE_WEBHOOK_SECRET`. The handler uses `webhook.ConstructEvent` from the official Stripe Go SDK.

### What Each Webhook Does in PocketBase

| Stripe Event | PocketBase Action |
|---|---|
| `checkout.session.completed` (subscription mode) | Creates/updates `stripe_subscriptions` record with status=active, period dates. Sets `users.tier = "pro"`. |
| `checkout.session.completed` (payment mode) | Retrieves line items. Adds `(quantity × credit_bundle_tokens)` to `user_limit_overrides.monthly_token_limit`. |
| `customer.subscription.created` | Creates `stripe_subscriptions` record mirroring Stripe state. Sets `users.tier = "pro"` if active. |
| `customer.subscription.updated` | Updates `stripe_subscriptions` record (status, period dates, cancel_at_period_end). Syncs `users.tier` based on status. |
| `customer.subscription.deleted` | Sets `stripe_subscriptions.status = "canceled"`, records `canceled_at`. Sets `users.tier = "free"`. |
| `invoice.payment_succeeded` | Refreshes `stripe_subscriptions` period dates from Stripe. Ensures tier stays synced. |
| `invoice.payment_failed` | Logs the failure. No immediate tier change. Stripe will fire `customer.subscription.updated` with `status=past_due`. |

### PocketBase Collections Affected

| Collection | Fields Updated | By Which Events |
|---|---|---|
| `stripe_customers` | `user_id`, `stripe_customer_id`, `email` | Created on first checkout (ensureCustomer) |
| `stripe_subscriptions` | `user_id`, `stripe_subscription_id`, `stripe_customer_id`, `stripe_price_id`, `status`, `current_period_start`, `current_period_end`, `cancel_at_period_end`, `canceled_at`, `cancel_reason` | All subscription events |
| `users` | `tier` ("free" or "pro") | subscription.created/updated/deleted, checkout.session.completed |
| `user_limit_overrides` | `monthly_token_limit` | checkout.session.completed (payment mode / credits) |

### Tier Sync Logic

The webhook handler maps Stripe subscription statuses to tiers:

- **Active / Trialing** → `users.tier = "pro"` (full Pro limits)
- **Canceled / Incomplete Expired / Unpaid** → `users.tier = "free"` (downgrade)
- **Past Due / Incomplete / Paused** → No tier change (Stripe handles dunning; user keeps Pro during grace period)

### Local Development

Use `STRIPE_SKIP_WEBHOOK_VERIFY=true` to accept unsigned webhook payloads during development.

---

## Buy Credits (Token Top-Up)

When a Pro subscriber exhausts their monthly token limit, they can purchase additional tokens without upgrading to a higher plan.

### How It Works

1. User calls `POST /api/stripe/buy-credits` with `quantity` (1–100 bundles)
2. Each bundle costs `credit_bundle_price` (€5) and adds `credit_bundle_tokens` (1,000,000) tokens
3. Tokens are added to `user_limit_overrides.monthly_token_limit`
4. The pricing system reads this override when checking limits:
   - Effective limit = base tier limit + purchased credits
5. Credits apply to the **current month only** — they do not roll over

### Guard Rails

- Only Pro subscribers can buy credits (free users must subscribe first)
- Minimum: 1 bundle, Maximum: 100 bundles per checkout
- Credits are cumulative within the same month (multiple purchases stack)

---

## Rate Limiting

Stripe enforces API [rate limits](https://docs.stripe.com/rate-limits). nannyapi handles `HTTP 429` responses transparently with **exponential back-off + full jitter**:

- Up to **4 attempts** per operation
- Initial delay: **500 ms**
- Maximum delay: **30 s**
- Each delay is randomised to avoid thundering-herd on concurrent requests

The SDK's own retry mechanism is disabled (set to 0) to avoid double-counting retries.

---

## Self-Hosted / No Stripe

Stripe is **fully optional**. Simply omit `STRIPE_SECRET_KEY` from your environment.

When not configured:
- All `/api/stripe/*` endpoints return HTTP `503` with `{"error": "payment integration is not configured on this instance"}`
- No Stripe SDK calls are made
- All other nannyapi features work normally on the free tier

You can manage tiers manually via the PocketBase admin UI (`users.tier` field) or via the admin pricing endpoints.

---

## Testing

### Unit Tests (no Stripe credentials needed)

```bash
go test ./internal/stripe/... -v
```

Covers: retry logic, backoff bounds, invoice parsing, client creation.

### Integration Tests (no Stripe credentials needed)

```bash
go test ./tests/... -run TestStripeManager -v
```

Covers: ErrNotConfigured for all methods, double-subscription guard, credits-require-subscription guard, quantity validation.

### End-to-End Tests (requires `sk_test_...`)

```bash
source .env
go test ./tests/... -run TestStripeE2E -v -count=1
```

Covers the **full lifecycle** against the real Stripe test API:
1. Create user in PocketBase
2. Create Stripe customer and attach test card (`tok_visa`)
3. Create subscription → verify tier upgrade to Pro
4. Verify double-subscription prevention
5. Cancel → verify `cancel_at_period_end=true`, user stays Pro
6. Reactivate → verify `cancel_at_period_end=false`
7. Buy credits → verify `user_limit_overrides.monthly_token_limit` increases
8. Quantity validation
9. Webhook dispatch: subscription.created, invoice.payment_succeeded, subscription.deleted → tier downgrade

### Test Tokens

| Token | Behaviour |
|---|---|
| `tok_visa` | Succeeds (4242 4242 4242 4242) |
| `tok_visa_debit` | Succeeds (debit card) |
| `tok_mastercard` | Succeeds (Mastercard) |
| `tok_chargeDeclined` | Declined |

See [Stripe Testing Docs](https://docs.stripe.com/testing) for more tokens.

### Cleanup After E2E Tests

E2E tests create real resources in your Stripe test account. Clean up via:
- **Stripe Dashboard** → Customers → delete test entries
- Or run `scripts/stripe-cleanup.sh` (archives products, deletes customers)
