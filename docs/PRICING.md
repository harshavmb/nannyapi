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
- Price: **$0/month**

### Pro Tier

| Resource | Daily Limit | Monthly Limit |
|----------|-------------|---------------|
| Agents | Unlimited | Unlimited |
| Tokens (input + output) | Unlimited | 10,000,000 |
| Investigations | Unlimited | Unlimited |

- Monthly limits reset on the **1st of each month**
- Price: **$10/month**
- Additional tokens can be purchased by contacting support@nannyai.dev

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

### Public Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/pricing` | Get public tier information |

### Authenticated Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/pricing/usage` | Get current user's usage and limits |

### Admin Endpoints

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/admin/pricing/promote` | Promote user to a tier |
| POST | `/api/admin/pricing/limits` | Set custom limits for a user |
| POST | `/api/admin/pricing/revoke` | Revoke a tier override |
| GET | `/api/admin/pricing/config` | Get full pricing configuration |
| POST | `/api/admin/pricing/config` | Update pricing configuration |
| GET | `/api/admin/pricing/user/{userId}` | Get user's full pricing details |

## Rate Limiting Behavior

When a user exceeds their limits, the API returns a `429 Too Many Requests` response with a descriptive error:

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

```json
{
  "code": "daily_investigation_limit_reached",
  "message": "Daily investigation limit of 5 reached. Resets at midnight.",
  "limit": 5,
  "used": 5,
  "resets_at": "2025-01-16T00:00:00Z"
}
```

### Error Codes

| Code | Description |
|------|-------------|
| `agent_limit_reached` | User has max agents for their tier |
| `daily_token_limit_reached` | Daily token budget exhausted |
| `monthly_token_limit_reached` | Monthly token budget exhausted |
| `daily_investigation_limit_reached` | Daily investigation quota used |
| `monthly_investigation_limit_reached` | Monthly investigation quota used |

## Admin Controls

### Promote User to Pro (Trial)

Allow a user to try Pro for a limited period:

```bash
curl -X POST /api/admin/pricing/promote \
  -H "Authorization: Bearer <admin_token>" \
  -d '{
    "user_id": "abc123",
    "tier": "pro",
    "duration_days": 14,
    "reason": "Trial period for evaluation"
  }'
```

Setting `duration_days` to `0` makes the promotion permanent.

### Set Custom Limits

Override specific limits for a user without changing their tier:

```bash
curl -X POST /api/admin/pricing/limits \
  -H "Authorization: Bearer <admin_token>" \
  -d '{
    "user_id": "abc123",
    "max_agents": 5,
    "monthly_token_limit": 5000000
  }'
```

Only fields provided will be overridden; unset fields use tier defaults.

### Revoke Override

Remove a tier promotion:

```bash
curl -X POST /api/admin/pricing/revoke \
  -H "Authorization: Bearer <admin_token>" \
  -d '{"user_id": "abc123"}'
```

## Database Collections

The pricing system uses the following collections:

| Collection | Purpose |
|------------|---------|
| `pricing_config` | Global pricing configuration (JSON) |
| `tier_overrides` | Admin-granted tier promotions (audit trail) |
| `user_limit_overrides` | Per-user custom limit values |
| `user_usage` | Tracks daily/monthly usage per user |

### `users.tier` Field

Every user has a `tier` field on the `users` collection that reflects their **current effective tier**. This field:

- **Defaults to `"free"`** for all new user registrations
- **Updates to `"pro"`** when an admin promotes a user (via `/api/admin/pricing/promote`)
- **Resets to `"free"`** when:
  - An admin revokes the override (via `/api/admin/pricing/revoke`)
  - A trial promotion expires (detected lazily on next tier check)

This field is the **source of truth visible in the PocketBase admin dashboard** and can be used to:
- Filter users by tier in the admin UI
- Display tier badges in a frontend
- Query user tiers via PocketBase's list API: `?filter=tier='pro'`

### Relationship: `users.tier` vs `tier_overrides`

| Aspect | `users.tier` | `tier_overrides` |
|--------|-------------|-----------------|
| Purpose | Current effective tier (UI/dashboard) | Full audit trail of promotions |
| Updated by | Pricing system automatically | Admin promote/revoke endpoints |
| Contains history | No (single value) | Yes (all past promotions) |
| Used for enforcement | Fallback only | Primary source for limit checks |
| Editable in dashboard | Yes (admin can manually set) | Yes (but use API for proper sync) |

The `GetUserTier()` resolution order:
1. Check `tier_overrides` for an **active, non-expired** override → use that tier
2. Fall back to `users.tier` field value
3. If empty or unknown → default to `"free"`

## Architecture Notes

- All limit values are stored in the database, **never hardcoded**
- The config file is read at startup and **auto-seeded into the `pricing_config` DB table** if empty (so admins can modify it from the dashboard)
- DB config can be hot-reloaded via admin API
- Self-hosted instances bypass all pricing checks entirely
- The pricing config file should **not** be committed to version control with real values (use `pricing.config.example.json` as a template)
- Admin operations are restricted to PocketBase superusers or users with `role=admin`
- Trial promotions auto-expire without requiring a cron job (checked on access)
- The `users.tier` field stays in sync with promotions/revocations automatically

## Running Tests

```bash
# All pricing tests (27 tests)
go test ./tests/ -run "TestPricing|TestFreeUser|TestProUser|TestAdmin|TestSelfHosted|TestUsageReset|TestGetUserUsage|TestPublicPricing|TestSaveAndReload|TestErrorMessages|TestUserTierField" -v

# Tier sync tests specifically
go test ./tests/ -run TestUserTierField -v
```
