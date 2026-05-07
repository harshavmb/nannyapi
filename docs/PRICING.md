# Pricing & Rate Limiting

NannyAPI implements a flexible, configurable pricing and rate-limiting system that supports both SaaS deployments and self-hosted instances.

## Overview

The pricing system is **opt-in** and **configuration-driven**. When no pricing configuration is provided (e.g., self-hosted deployments), all limits are disabled and users have unrestricted access. This ensures self-hosted users are never impacted by pricing logic.

## Tiers

### Free Tier

| Resource | Daily Limit | Monthly Limit |
|----------|-------------|---------------|
| Agents | 2 per user | 2 per user |
| Tokens (input + output) | 200,000 | 1,000,000 |
| Investigations | 5 | 25 |

- Daily limits reset at **midnight** every day
- Monthly limits reset on the **1st of each month**
- Price: **€0/month**

### Pro Tier

| Resource | Daily Limit | Monthly Limit |
|----------|-------------|---------------|
| Agents | Unlimited | Unlimited |
| Tokens (input + output) | Unlimited | 10,000,000 |
| Investigations | Unlimited | Unlimited |

- Monthly limits reset on the **1st of each month**
- Price: **€10/month**
- Additional tokens available via **Buy Credits** (see below)

### Credit Bundles (Pro Tier Add-on)

Pro subscribers who exhaust their monthly 10M token allowance can purchase additional tokens:

| Bundle | Tokens | Price |
|--------|--------|-------|
| Credit Pack | 1,000,000 | €5 (one-time) |

- Credits are added to `user_limit_overrides.monthly_token_limit` immediately on successful payment
- Purchase via `POST /api/stripe/buy-credits` (requires active Pro subscription)
- All values are defined in `pricing.config.json` (`credit_bundle_tokens`, `credit_bundle_price`)

### Enterprise / Custom

For requirements beyond the Pro tier, contact **support@nannyai.dev**.

## Configuration

### Enabling Pricing

Pricing can be enabled via two methods:

#### Method 1: Configuration File (Recommended for deployment)

Set the `NANNYAPI_PRICING_CONFIG` environment variable to point to a JSON configuration file:

```bash
export NANNYAPI_PRICING_CONFIG=/path/to/pricing.json
```

Example config file (`pricing.config.example.json` in repo root):

```json
{
  "enabled": true,
  "currency": "eur",
  "credit_bundle_tokens": 1000000,
  "credit_bundle_price": 5,
  "tiers": {
    "free": {
      "name": "free",
      "max_agents": 2,
      "daily_token_limit": 200000,
      "monthly_token_limit": 1000000,
      "daily_investigation_limit": 5,
      "monthly_investigation_limit": 25,
      "price_per_month": 0,
      "contact_email": "support@nannyai.dev"
    },
    "pro": {
      "name": "pro",
      "max_agents": -1,
      "daily_token_limit": -1,
      "monthly_token_limit": 10000000,
      "daily_investigation_limit": -1,
      "monthly_investigation_limit": -1,
      "price_per_month": 10,
      "contact_email": "support@nannyai.dev"
    }
  }
}
```

> **Note:** Use `-1` to indicate "unlimited" for any limit field.

#### Method 2: Database Configuration (Runtime updates via Admin API)

The admin can update pricing config at runtime via:

```
POST /api/admin/pricing/config
```

This stores the configuration in the `pricing_config` collection and takes effect immediately.

### Disabling Pricing (Self-Hosted)

Simply do **not** set `NANNYAPI_PRICING_CONFIG` and do not create a `pricing_config` record in the database. The system defaults to disabled, giving all users unlimited access.

## API Endpoints

### Summary

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/api/pricing` | None | Get public tier information |
| GET | `/api/pricing/usage` | User | Get current user's usage and limits |
| POST | `/api/admin/pricing/promote` | Admin | Promote user to a tier |
| POST | `/api/admin/pricing/limits` | Admin | Set custom limits for a user |
| POST | `/api/admin/pricing/revoke` | Admin | Revoke a tier override |
| GET | `/api/admin/pricing/config` | Admin | Get full pricing configuration |
| POST | `/api/admin/pricing/config` | Admin | Update pricing configuration |
| GET | `/api/admin/pricing/user/{userId}` | Admin | Get user's full pricing details |

---

### `GET /api/pricing`

Returns public tier information. No authentication required.

**Response (pricing enabled):**

```json
{
  "enabled": true,
  "tiers": [
    {
      "name": "free",
      "max_agents": 2,
      "daily_token_limit": 200000,
      "monthly_token_limit": 1000000,
      "daily_investigation_limit": 5,
      "monthly_investigation_limit": 25,
      "price_per_month": 0,
      "contact_email": "support@nannyai.dev"
    },
    {
      "name": "pro",
      "max_agents": -1,
      "daily_token_limit": -1,
      "monthly_token_limit": 10000000,
      "daily_investigation_limit": -1,
      "monthly_investigation_limit": -1,
      "price_per_month": 10,
      "contact_email": "support@nannyai.dev"
    }
  ]
}
```

> **Note:** Tiers are always returned in deterministic order: `free` first, then `pro`.

**Response (pricing disabled / self-hosted):**

```json
{
  "enabled": false,
  "message": "Pricing is not enabled on this instance"
}
```

---

### `GET /api/pricing/usage`

Returns the authenticated user's current tier, limits, and usage counters.

**Headers:** `Authorization: Bearer <user_token>`

**Response (pricing enabled):**

```json
{
  "enabled": true,
  "usage": {
    "tier": "pro",
    "max_agents": -1,
    "daily_token_limit": -1,
    "monthly_token_limit": 10000000,
    "daily_investigation_limit": -1,
    "monthly_investigation_limit": -1,
    "daily_tokens_used": 42000,
    "monthly_tokens_used": 850000,
    "daily_investigations_used": 2,
    "monthly_investigations_used": 12,
    "daily_resets_at": "2026-05-01T00:00:00+02:00",
    "monthly_resets_at": "2026-06-01T00:00:00+02:00"
  }
}
```

**Response (pricing disabled):**

```json
{
  "enabled": false,
  "message": "No usage limits on this instance"
}
```

**Limit value semantics:**

| Value | Meaning |
|-------|---------|
| `-1` | Unlimited (no enforcement for this resource) |
| `0` | Blocked (resource fully restricted) |
| `> 0` | Hard limit — usage is denied when reached |

---

### `POST /api/admin/pricing/promote`

Promote a user to a specific tier. Creates a new tier override and deactivates any existing active override for that user.

**Headers:** `Authorization: Bearer <admin_token>`

**Request:**

```json
{
  "user_id": "bxjz20ww7ilz1p6",
  "tier": "pro",
  "duration_days": 14,
  "reason": "Trial period for evaluation"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `user_id` | string | Yes | PocketBase user record ID |
| `tier` | string | Yes | Target tier: `"free"` or `"pro"` |
| `duration_days` | int | No | Trial duration. `0` = permanent |
| `reason` | string | No | Audit reason for the promotion |

**Response (200):**

```json
{
  "message": "user tier updated successfully"
}
```

**Behavior:**
- Any existing **active** tier override for the user is **deactivated** (set `active=false`, `updated` timestamp updated)
- A new `tier_overrides` record is created with `active=true`
- The `users.tier` field is synced to the new tier value
- Only **one active override per user** is allowed (enforced by unique partial index)
- The `created` and `updated` timestamps are auto-populated

---

### `POST /api/admin/pricing/limits`

Set custom per-user limit overrides without changing their tier. Only provided fields are overridden; omitted fields use tier defaults.

**Headers:** `Authorization: Bearer <admin_token>`

**Request:**

```json
{
  "user_id": "bxjz20ww7ilz1p6",
  "max_agents": 5,
  "daily_token_limit": 500000,
  "monthly_token_limit": 5000000,
  "daily_investigation_limit": 10,
  "monthly_investigation_limit": 50
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `user_id` | string | Yes | PocketBase user record ID |
| `max_agents` | int | No | Override max agents (`-1` = unlimited, `0` = block) |
| `daily_token_limit` | int64 | No | Override daily token limit |
| `monthly_token_limit` | int64 | No | Override monthly token limit |
| `daily_investigation_limit` | int | No | Override daily investigation limit |
| `monthly_investigation_limit` | int | No | Override monthly investigation limit |

**Limit override semantics:**

| Value | Meaning |
|-------|---------|
| `-1` | Use tier default (override not applied) |
| `0` | Block — resource usage denied completely |
| `> 0` | Custom hard limit |

**Response (200):**

```json
{
  "message": "user limits updated successfully"
}
```

---

### `POST /api/admin/pricing/revoke`

Revoke an active tier override for a user. Deactivates all active overrides and resets `users.tier` to `"free"`.

**Headers:** `Authorization: Bearer <admin_token>`

**Request:**

```json
{
  "user_id": "bxjz20ww7ilz1p6"
}
```

**Response (200):**

```json
{
  "message": "tier override revoked"
}
```

**Behavior:**
- All active `tier_overrides` for the user are set to `active=false`
- The `users.tier` field is reset to `"free"`
- The `updated` timestamp on deactivated records is updated

---

### `GET /api/admin/pricing/config`

Returns the full pricing configuration (tier definitions and limits).

**Headers:** `Authorization: Bearer <admin_token>`

**Response (200):**

```json
{
  "enabled": true,
  "tiers": {
    "free": {
      "name": "free",
      "max_agents": 2,
      "daily_token_limit": 200000,
      "monthly_token_limit": 1000000,
      "daily_investigation_limit": 5,
      "monthly_investigation_limit": 25,
      "price_per_month": 0,
      "contact_email": "support@nannyai.dev"
    },
    "pro": {
      "name": "pro",
      "max_agents": -1,
      "daily_token_limit": -1,
      "monthly_token_limit": 10000000,
      "daily_investigation_limit": -1,
      "monthly_investigation_limit": -1,
      "price_per_month": 10,
      "contact_email": "support@nannyai.dev"
    }
  }
}
```

---

### `POST /api/admin/pricing/config`

Update the pricing configuration at runtime. Takes effect immediately.

**Headers:** `Authorization: Bearer <admin_token>`

**Request:** Same structure as the response from `GET /api/admin/pricing/config`.

**Response (200):**

```json
{
  "message": "pricing config updated"
}
```

---

### `GET /api/admin/pricing/user/{userId}`

Get full pricing details for a specific user: their effective tier, current usage, and applicable limits.

**Headers:** `Authorization: Bearer <admin_token>`

**Response (200):**

```json
{
  "tier": "pro",
  "usage": {
    "tier": "pro",
    "max_agents": -1,
    "daily_token_limit": -1,
    "monthly_token_limit": 10000000,
    "daily_investigation_limit": -1,
    "monthly_investigation_limit": -1,
    "daily_tokens_used": 42000,
    "monthly_tokens_used": 850000,
    "daily_investigations_used": 2,
    "monthly_investigations_used": 12,
    "daily_resets_at": "2026-05-01T00:00:00+02:00",
    "monthly_resets_at": "2026-06-01T00:00:00+02:00"
  },
  "limits": {
    "max_agents": -1,
    "daily_token_limit": -1,
    "monthly_token_limit": 10000000,
    "daily_investigation_limit": -1,
    "monthly_investigation_limit": -1
  }
}
```

---

## Rate Limiting Behavior

When a user exceeds their limits, the API returns a `429 Too Many Requests` response:

```json
{
  "code": "agent_limit_reached",
  "message": "You have reached the maximum number of agents (2) for your plan. Upgrade to Pro for unlimited agents.",
  "limit": 2,
  "used": 2,
  "resets_at": ""
}
```

```json
{
  "code": "daily_token_limit_reached",
  "message": "Daily token limit of 200000 reached. Resets at midnight.",
  "limit": 200000,
  "used": 195000,
  "resets_at": "2025-01-16T00:00:00Z"
}
```

### Error Codes

| Code | HTTP Status | Description |
|------|-------------|-------------|
| `agent_limit_reached` | 429 | User has max agents for their tier |
| `daily_token_limit_reached` | 429 | Daily token budget exhausted |
| `monthly_token_limit_reached` | 429 | Monthly token budget exhausted |
| `daily_investigation_limit_reached` | 429 | Daily investigation quota used |
| `monthly_investigation_limit_reached` | 429 | Monthly investigation quota used |

### Error Response Format (all endpoints)

```json
{
  "error": "description of what went wrong"
}
```

---

## Database Collections

The pricing system uses the following collections:

| Collection | Purpose |
|------------|---------|
| `pricing_config` | Global pricing configuration (JSON) |
| `tier_overrides` | Admin-granted tier promotions (full audit trail) |
| `user_limit_overrides` | Per-user custom limit values |
| `user_usage` | Tracks daily/monthly usage counters per user |

### `tier_overrides` Schema

| Field | Type | Description |
|-------|------|-------------|
| `id` | text | PocketBase auto-generated record ID |
| `user_id` | relation | FK to `users` collection (cascade delete) |
| `tier` | text | The granted tier (`"free"` or `"pro"`) |
| `granted_by` | text | Admin user ID who granted the override |
| `reason` | text | Human-readable reason for audit trail |
| `active` | bool | Whether this override is currently active |
| `expires_at` | date | When the override expires (empty = permanent) |
| `created` | autodate | Timestamp when the record was created |
| `updated` | autodate | Timestamp when the record was last modified |

**Constraints:**
- Only **one active override per user** (enforced by unique partial index `idx_tier_overrides_user_active WHERE active = 1`)
- When a new promote is issued, existing active overrides are deactivated first
- Deactivated records are preserved as audit history (never deleted)

### `user_usage` Schema

| Field | Type | Description |
|-------|------|-------------|
| `id` | text | PocketBase auto-generated record ID |
| `user_id` | relation | FK to `users` collection (cascade delete, unique) |
| `daily_tokens_used` | number | Tokens consumed today |
| `monthly_tokens_used` | number | Tokens consumed this month |
| `daily_investigations_used` | number | Investigations today |
| `monthly_investigations_used` | number | Investigations this month |
| `daily_reset_at` | date | When daily counters reset next |
| `monthly_reset_at` | date | When monthly counters reset next |
| `created` | autodate | Timestamp when the record was created |
| `updated` | autodate | Timestamp when the record was last modified |

**Constraints:**
- One record per user (enforced by unique index `idx_user_usage_user_id`)
- Counters reset automatically on access when the reset time is past

### `user_limit_overrides` Schema

| Field | Type | Description |
|-------|------|-------------|
| `id` | text | PocketBase auto-generated record ID |
| `user_id` | relation | FK to `users` collection (cascade delete) |
| `max_agents` | number | Custom agent limit (`-1` = use tier default, `0` = block) |
| `daily_token_limit` | number | Custom daily token limit |
| `monthly_token_limit` | number | Custom monthly token limit |
| `daily_investigation_limit` | number | Custom daily investigation limit |
| `monthly_investigation_limit` | number | Custom monthly investigation limit |

### `users.tier` Field

Every user has a `tier` field on the `users` collection that reflects their **current effective tier**:

- **Defaults to `"free"`** for all new registrations
- **Updates to `"pro"`** when an admin promotes a user
- **Resets to `"free"`** on revoke or trial expiry

This field is the **source of truth for the UI/dashboard** and can be used to:
- Filter users by tier: `?filter=tier='pro'`
- Display tier badges in frontends
- Query in PocketBase admin dashboard

### Tier Resolution Order

The `GetUserTier()` function resolves the effective tier as follows:
1. Check `tier_overrides` for an **active, non-expired** override → use that tier
2. Fall back to `users.tier` field value
3. If empty or unknown → default to `"free"`

### Relationship: `users.tier` vs `tier_overrides`

| Aspect | `users.tier` | `tier_overrides` |
|--------|-------------|-----------------|
| Purpose | Current effective tier (UI/dashboard) | Full audit trail of promotions |
| Updated by | Pricing system automatically | Admin promote/revoke endpoints |
| Contains history | No (single value) | Yes (all past promotions) |
| Used for enforcement | Fallback only | Primary source for limit checks |
| Editable in dashboard | Yes (admin can manually set) | Yes (but use API for proper sync) |

---

## Integration Guide (for Agents & UI)

### Checking If Pricing Is Enabled

```
GET /api/pricing
```

If `enabled` is `false`, skip all pricing/usage UI. If `true`, use the `tiers` array to render plan comparisons.

### Displaying User Usage

```
GET /api/pricing/usage
Authorization: Bearer <user_token>
```

Use the `usage` object to render:
- Progress bars for token/investigation usage
- Current tier badge
- Reset countdown timers from `daily_resets_at` / `monthly_resets_at`

### Handling Rate Limit Errors

When any agent API call returns `429`, parse the JSON body:

```json
{
  "code": "monthly_token_limit_reached",
  "message": "Monthly token limit reached...",
  "limit": 1000000,
  "used": 1000000,
  "resets_at": "2026-06-01T00:00:00Z"
}
```

Display the `message` to the user and optionally show an upgrade CTA.

### Admin: Managing User Tiers

1. **Promote:** `POST /api/admin/pricing/promote` → creates override, syncs `users.tier`
2. **Check:** `GET /api/admin/pricing/user/{userId}` → see effective tier + usage
3. **Revoke:** `POST /api/admin/pricing/revoke` → deactivates all overrides, resets to free
4. **Custom limits:** `POST /api/admin/pricing/limits` → fine-tune without tier change

### Timestamps & Audit Trail

Every `tier_overrides` record has `created` and `updated` autodate fields:
- `created`: When the override was initially created (ISO 8601 UTC)
- `updated`: When the override was last modified (e.g., deactivated)

Use these for:
- Audit logs showing when promotions were granted/revoked
- Sorting override history by recency
- Calculating time-on-tier metrics

---

## Architecture Notes

- All limit values are stored in the database, **never hardcoded**
- The config file is read at startup and **auto-seeded into the `pricing_config` DB table** if empty
- DB config can be hot-reloaded via admin API
- Self-hosted instances bypass all pricing checks entirely
- Admin operations are restricted to PocketBase superusers (`_superusers` collection)
- Trial promotions auto-expire without requiring a cron job (checked lazily on access)
- The `users.tier` field stays in sync with promotions/revocations automatically
- Concurrent usage updates are protected by per-user mutexes (no lost increments)
- Usage record creation uses double-check locking to prevent duplicates under concurrency

## Running Tests

```bash
# All pricing tests
go test ./tests/ -run "TestPricing|TestFreeUser|TestProUser|TestAdmin|TestSelfHosted|TestUsageReset|TestGetUserUsage|TestPublicPricing|TestSaveAndReload|TestErrorMessages|TestUserTierField|TestLimitOverride|TestSingleActiveTierOverride|TestTierOverridesHaveTimestamps|TestGetPublicInfoDeterministicOrder|TestConcurrentUsageCreation|TestUserUsageUniqueIndex" -v

# Tier override & deduplication tests
go test ./tests/ -run "TestSingleActiveTierOverride|TestTierOverridesHaveTimestamps" -v

# Concurrency tests
go test ./tests/ -run "TestConcurrentUsageCreation" -v
```
