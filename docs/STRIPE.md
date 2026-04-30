# Stripe Payment Integration

nannyapi supports optional billing via [Stripe](https://stripe.com). When the integration is not configured, all payment endpoints return a clear `402 Payment Required` response so self-hosted users experience a clean error rather than a crash.

---

## Table of Contents

1. [Quick Start](#quick-start)
2. [Environment Variables](#environment-variables)
3. [Stripe Dashboard Setup](#stripe-dashboard-setup)
4. [API Endpoints](#api-endpoints)
5. [Subscription Lifecycle](#subscription-lifecycle)
6. [Webhooks](#webhooks)
7. [Rate Limiting](#rate-limiting)
8. [Self-Hosted / No Stripe](#self-hosted--no-stripe)
9. [Testing](#testing)

---

## Quick Start

```bash
# 1. Copy your Stripe keys from https://dashboard.stripe.com/apikeys
export STRIPE_SECRET_KEY="sk_live_..."
export STRIPE_PUBLISHABLE_KEY="pk_live_..."

# 2. Create a monthly recurring price in the Stripe Dashboard and copy its ID
export STRIPE_PRO_PRICE_ID="price_..."

# 3. (Optional) Create a one-time credits price for Pay-As-You-Go top-ups
export STRIPE_CREDITS_PRICE_ID="price_..."

# 4. Set the webhook secret (see Webhooks section below)
export STRIPE_WEBHOOK_SECRET="whsec_..."

# 5. Run nannyapi
./nannyapi serve
```

---

## Environment Variables

| Variable | Required | Description |
|---|---|---|
| `STRIPE_SECRET_KEY` | Yes (to enable) | Stripe secret key (`sk_live_…` or `sk_test_…`). **Omit entirely to disable Stripe on self-hosted installs.** |
| `STRIPE_PUBLISHABLE_KEY` | No | Publishable key for client-side use (returned by `/api/stripe/subscription`). |
| `STRIPE_PRO_PRICE_ID` | Yes (if enabled) | Stripe Price ID for the monthly Pro plan subscription. |
| `STRIPE_CREDITS_PRICE_ID` | Yes (for top-ups) | Stripe Price ID for a one-time credit bundle (Pay-As-You-Go). |
| `STRIPE_WEBHOOK_SECRET` | Yes (production) | Webhook signing secret from your Stripe endpoint configuration. |
| `STRIPE_SKIP_WEBHOOK_VERIFY` | Dev only | Set to `"true"` to skip webhook signature verification. **Never use in production.** |

---

## Stripe Dashboard Setup

### 1. Create Products and Prices

**Pro Plan (subscription)**

1. Go to **Products → Add Product**.
2. Name it `nannyapi Pro`.
3. Add a **recurring price** (e.g., $29/month). Copy the **Price ID** (`price_…`) → `STRIPE_PRO_PRICE_ID`.

**Credit Bundle (one-time, optional)**

1. Add another product or price to the same product as a one-time payment.
2. Copy the **Price ID** → `STRIPE_CREDITS_PRICE_ID`.

### 2. Register a Webhook Endpoint

1. Go to **Developers → Webhooks → Add endpoint**.
2. Endpoint URL: `https://your-domain.com/api/stripe/webhook`
3. Select the following events to listen for:
   - `checkout.session.completed`
   - `customer.subscription.created`
   - `customer.subscription.updated`
   - `customer.subscription.deleted`
   - `invoice.payment_succeeded`
   - `invoice.payment_failed`
4. Copy the **Signing Secret** (`whsec_…`) → `STRIPE_WEBHOOK_SECRET`.

---

## API Endpoints

All endpoints require authentication (`Authorization: Bearer <token>`).

### `GET /api/stripe/subscription`

Returns the current user's subscription status.

**Response (200)**
```json
{
  "configured": true,
  "publishable_key": "pk_live_...",
  "subscription": {
    "id": "rec_...",
    "status": "active",
    "stripe_subscription_id": "sub_...",
    "current_period_start": "2025-01-01T00:00:00Z",
    "current_period_end": "2025-02-01T00:00:00Z",
    "cancel_at_period_end": false
  }
}
```

When not configured:
```json
{ "configured": false }
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

Redirect the user to `checkout_url`. Stripe handles card entry, authentication, and confirmation.

**Errors**
- `402` – Stripe not configured
- `409` – User already has an active subscription (use `/buy-credits` for top-ups)

---

### `POST /api/stripe/cancel-subscription`

Schedules the active subscription to cancel at the end of the current billing period. The user remains Pro until that date.

**Request**
```json
{
  "reason": "too expensive"
}
```

**Response (200)** `{}`

---

### `POST /api/stripe/reactivate-subscription`

Removes the pending cancellation on a subscription that was scheduled to cancel.

**Response (200)** `{}`

---

### `POST /api/stripe/buy-credits`

Creates a one-time Stripe Checkout Session for extra token bundles (Pay-As-You-Go).

**Requires an active Pro subscription.** A credit bundle adds 1 000 000 tokens to the user's monthly allowance.

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
- `402` – Stripe not configured
- `403` – No active Pro subscription
- `400` – Invalid quantity (must be 1–100)

---

### `POST /api/stripe/webhook`

Receives Stripe webhook events. This endpoint is called by Stripe, not by the app frontend. The signature is verified using `STRIPE_WEBHOOK_SECRET`.

**Events handled**

| Event | Action |
|---|---|
| `checkout.session.completed` (subscription) | Activates subscription, upgrades user tier to Pro |
| `checkout.session.completed` (payment) | Grants extra token credits to user |
| `customer.subscription.created` | Syncs subscription to local DB |
| `customer.subscription.updated` | Syncs subscription status changes |
| `customer.subscription.deleted` | Marks subscription canceled, downgrades user to Free |
| `invoice.payment_succeeded` | Refreshes subscription period dates on renewal |
| `invoice.payment_failed` | Logs the failure (Stripe sends `customer.subscription.updated` with `past_due` status) |

---

## Subscription Lifecycle

```
User clicks "Subscribe"
  → POST /api/stripe/subscribe
  → Redirected to Stripe Checkout
  → Stripe sends checkout.session.completed
  → User tier → Pro ✓

User clicks "Cancel"
  → POST /api/stripe/cancel-subscription
  → Subscription.cancel_at_period_end = true
  → User remains Pro until period end
  → Stripe sends customer.subscription.deleted at period end
  → User tier → Free

User clicks "Reactivate"
  → POST /api/stripe/reactivate-subscription
  → Subscription.cancel_at_period_end = false
  → User remains Pro

User clicks "Buy Credits" (Pro only)
  → POST /api/stripe/buy-credits
  → Redirected to Stripe Checkout (one-time payment)
  → Stripe sends checkout.session.completed
  → Extra tokens added to monthly allowance
```

---

## Webhooks

Webhook events are verified using the `Stripe-Signature` header and `STRIPE_WEBHOOK_SECRET`. The signature check uses `webhook.ConstructEvent` from the official Stripe Go SDK.

**Development without a public URL:** Use the [Stripe CLI](https://docs.stripe.com/stripe-cli) to forward events locally:

```bash
stripe listen --forward-to localhost:8090/api/stripe/webhook
# The CLI prints a webhook signing secret – use it as STRIPE_WEBHOOK_SECRET
```

---

## Rate Limiting

Stripe enforces API [rate limits](https://docs.stripe.com/rate-limits). nannyapi handles `HTTP 429` responses transparently with **exponential back-off + full jitter**:

- Up to **4 attempts** per operation
- Initial delay: **500 ms**
- Maximum delay: **30 s**
- Each delay is randomised to avoid thundering-herd on concurrent requests

A log message is emitted on each retry:

```
[stripe] rate limited (attempt 1/4): Too Many Requests - retrying in 412ms
```

The SDK's own retry mechanism is disabled (set to 0) to avoid double-counting retries.

---

## Self-Hosted / No Stripe

Stripe is **fully optional**. Simply omit `STRIPE_SECRET_KEY` from your environment.

When not configured:
- All `/api/stripe/*` endpoints return HTTP `402` with `{"error": "stripe payment integration is not configured on this instance"}`
- No Stripe SDK calls are made
- `GET /api/stripe/subscription` returns `{"configured": false}`
- All other nannyapi features work normally

This means your users will be on the free tier by default. You can manage tiers manually via the PocketBase admin UI or a custom integration.

---

## Testing

### Unit tests

```bash
go test ./internal/stripe/...
```

Tests cover:
- `retryDo` back-off logic (success, non-retryable error, rate-limit retry)
- `backoffDelay` bounds
- `extractInvoiceSubscriptionID` for all nil-check paths
- `newStripeClient` with and without `STRIPE_SECRET_KEY`

### Integration tests

```bash
go test ./tests/... -run TestStripe
```

Tests cover:
- All Manager methods return `ErrNotConfigured` without `STRIPE_SECRET_KEY`
- `GetSubscription` / `HasActiveSubscription` for unknown users
- Double-subscription prevention
- Credit purchase requires active subscription
- Quantity validation

### End-to-end with Stripe test mode

Use Stripe [test cards](https://docs.stripe.com/testing):

| Card | Behaviour |
|---|---|
| `4242 4242 4242 4242` | Succeeds |
| `4000 0025 0000 3155` | Requires 3D Secure authentication |
| `4000 0000 0000 9995` | Declined (insufficient funds) |

Set `STRIPE_SECRET_KEY=sk_test_…` and `STRIPE_PUBLISHABLE_KEY=pk_test_…` from your [Stripe Dashboard test mode](https://dashboard.stripe.com/test/apikeys).
