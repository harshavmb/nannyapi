# API Reference

<p align="center">
  <img src="https://avatars.githubusercontent.com/u/110624612" alt="NannyAgent Logo" width="120" />
</p>

Complete reference for all NannyAPI endpoints with actual request/response examples.

## Base URL
```text
http://<your-server>:8090
```

## Authentication

### User Authentication
Bearer token obtained via OAuth2 or Email/Password login:
```text
Authorization: Bearer <user_token>
```

### Agent Authentication
Bearer token obtained via device authorization flow:
```text
Authorization: Bearer <agent_token>
```

---

## Agent Management

All agent operations use a single endpoint with different `action` values.

### Base Endpoint
```text
POST /api/agent
```

### 1. Device Auth Start (Anonymous)

Initiates device authorization flow. Agent calls this without authentication.

**Request:**
```json
{
  "action": "device-auth-start"
}
```

**Response (200 OK):**
```json
{
  "device_code": "a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6",
  "user_code": "ABCD1234",
  "verification_uri": "http://localhost:8080/agent/authorize?user_code=ABCD1234",
  "expires_in": 600
}
```

**Notes:**
- Device code: 36-character UUID
- User code: 8-character alphanumeric (no ambiguous characters)
- Expires in 10 minutes (600 seconds)
- Frontend URL configured via `FRONTEND_URL` environment variable

---

### 2. Authorize Device (User)

User authorizes a pending device code via web interface.

**Authentication:** Required (User)

**Request:**
```json
{
  "action": "authorize",
  "user_code": "ABCD1234"
}
```

**Response (200 OK):**
```json
{
  "success": true,
  "message": "device authorized"
}
```

**Errors:**
- `400`: Invalid or expired user code
- `401`: Authentication required

---

### 3. Register Agent

Agent completes registration after user authorization.

**Request:**
```json
{
  "action": "register",
  "device_code": "a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6",
  "hostname": "prod-server-01",
  "os_type": "linux",
  "os_info": "Ubuntu 22.04.3 LTS",
  "os_version": "22.04",
  "platform_family": "debian",
  "version": "1.0.0",
  "kernel_version": "5.15.0-91-generic",
  "arch": "amd64",
  "primary_ip": "192.168.1.100",
  "all_ips": ["192.168.1.100", "10.0.0.5", "172.17.0.1"]
}
```

**Response (200 OK):**
```json
{
  "access_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "refresh_token": "f7e8d9c0b1a2938475869faebdcc0123",
  "expires_in": 3600,
  "agent_id": "abc123xyz456"
}
```

**Field Descriptions:**
- `os_type`: linux (only supported OS currently)
- `os_info`: Human-readable OS description
- `platform_family`: debian, rhel, arch, suse (only supported families currently)
- `arch`: amd64, arm64, etc.
- `primary_ip`: Primary network interface IP (e.g., eth0, WAN)
- `all_ips`: All IP addresses from all network interfaces
- `refresh_token`: Used to obtain new access tokens (SHA-256 stored, expires in 30 days)

**Supported Platform Families:**
- `debian` - Debian, Ubuntu, Linux Mint, Pop!_OS
- `rhel` - RHEL, CentOS, Fedora, Rocky Linux, AlmaLinux
- `arch` - Arch Linux, Manjaro, EndeavourOS
- `suse` - SUSE, openSUSE

> **Note**: Additional Linux distributions may be added based on user requests.

**Platform Family Auto-Detection:**
If `platform_family` is empty, API attempts to guess from `os_info`:
- "ubuntu", "debian", "mint" → `debian`
- "red hat", "rhel", "centos", "fedora", "alma", "rocky" → `rhel`
- "suse", "sles" → `suse`
- "arch", "manjaro" → `arch`

**Errors:**
- `400`: Invalid/expired device code, device not authorized, or device already used

---

### 4. Refresh Access Token

Obtain a new **access token** using the current refresh token. The refresh
token itself is **NOT rotated** by this call — the same refresh token
remains valid until its 30-day expiry. This is the standard, high-frequency
flow agents use whenever their access token is about to expire.

To rotate the refresh token itself, use the separate
[`renew-refresh-token`](#5-renew-refresh-token) action below.

**Authentication:** None (the refresh token in the body is the credential).

**Request:**
```json
{
  "action": "refresh",
  "refresh_token": "f7e8d9c0b1a2938475869faebdcc0123"
}
```

**Response (200 OK):**
```json
{
  "access_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "expires_in": 3600,
  "refresh_token_expires_in": 2591400,
  "agent_id": "abc123xyz456"
}
```

**Field Descriptions:**
- `access_token`: Short-lived bearer token for agent API calls.
- `expires_in`: Access-token lifetime in seconds (1 hour).
- `refresh_token_expires_in`: Remaining lifetime of the **current**
  refresh token in seconds. When this approaches zero, the agent should
  call `renew-refresh-token` to obtain a fresh refresh token.
- `agent_id`: Identifier of the agent this token belongs to.
- `refresh_token`: **Not returned** — the existing refresh token is
  unchanged and MUST continue to be used.

**Errors:**
- `400`: `refresh_token required`
- `401`: `invalid refresh token` (unknown or revoked)
- `401`: `refresh token expired`
- `401`: `agent revoked`

---

### 5. Renew Refresh Token

Explicitly **rotate** the refresh token. The previous refresh token is
invalidated immediately and replaced with the one returned here. Agents
should call this sparingly — typically only when `refresh_token_expires_in`
from the `refresh` action is approaching zero (e.g. < 24 hours remaining).

The response also includes a fresh access token so the agent can resume
normal operation without an extra round-trip.

**Authentication:** None (the refresh token in the body is the credential).

**Request:**
```json
{
  "action": "renew-refresh-token",
  "refresh_token": "f7e8d9c0b1a2938475869faebdcc0123"
}
```

**Response (200 OK):**
```json
{
  "access_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "refresh_token": "9a8b7c6d5e4f3a2b1c0d9e8f7a6b5c4d",
  "expires_in": 3600,
  "refresh_token_expires_in": 2592000,
  "agent_id": "abc123xyz456"
}
```

**Field Descriptions:**
- `refresh_token`: **New** refresh token — persist it and discard the old
  one *before* using the new access token. The old refresh token stops
  working the instant the server commits the rotation.
- `access_token`, `expires_in`, `refresh_token_expires_in`, `agent_id`:
  as described above.

**Errors:**
- `400`: `refresh_token required`
- `401`: `invalid refresh token`
- `401`: `refresh token expired`
- `401`: `agent revoked`

---

### 6. Ingest Metrics

Agent reports system metrics (typically every 30 seconds).

**Authentication:** Required (Agent)

**Request:**
```json
{
  "action": "ingest-metrics",
  "system_metrics": {
    "cpu_percent": 45.2,
    "cpu_cores": 8,
    "memory_used_gb": 12.5,
    "memory_total_gb": 16.0,
    "memory_percent": 78.1,
    "disk_used_gb": 250.0,
    "disk_total_gb": 500.0,
    "disk_usage_percent": 50.0,
    "load_average": {
      "one_min": 2.15,
      "five_min": 1.98,
      "fifteen_min": 1.75
    },
    "network_stats": {
      "in_gb": 125.5,
      "out_gb": 89.3
    },
    "filesystems": [
      {
        "device": "/dev/sda1",
        "mount_path": "/",
        "used_gb": 200.0,
        "free_gb": 280.0,
        "total_gb": 480.0,
        "usage_percent": 41.7
      },
      {
        "device": "/dev/sdb1",
        "mount_path": "/data",
        "used_gb": 50.0,
        "free_gb": 950.0,
        "total_gb": 1000.0,
        "usage_percent": 5.0
      }
    ]
  },
  "os_info": "Ubuntu 22.04.3 LTS",
  "os_version": "22.04",
  "version": "1.0.1",
  "primary_ip": "192.168.1.100",
  "kernel_version": "5.15.0-91-generic",
  "platform_family": "debian",
  "arch": "amd64",
  "all_ips": ["192.168.1.100", "10.0.0.5"]
}
```

**Response (200 OK):**
```json
{
  "success": true,
  "message": "metrics recorded"
}
```

**Notes:**
- Metrics update `agent_metrics` collection (upsert based on `agent_id`)
- Also updates agent's `last_seen` timestamp and metadata fields
- Metrics are stored as individual fields for easy querying and dashboard visualization
- `filesystems` stored as JSON text for per-mount statistics

**Errors:**
- `401`: Authentication required
- `403`: Agent revoked

---

### 7. List Agents (User)

Retrieve all agents owned by authenticated user.

**Authentication:** Required (User)

**Request:**
```json
{
  "action": "list"
}
```

**Response (200 OK):**
```json
{
  "agents": [
    {
      "id": "abc123xyz456",
      "hostname": "prod-server-01",
      "os_type": "linux",
      "os_info": "Ubuntu 22.04.3 LTS",
      "os_version": "22.04",
      "version": "1.0.1",
      "status": "active",
      "health": "healthy",
      "last_seen": "2024-01-15T10:30:45Z",
      "created": "2024-01-01T08:00:00Z",
      "kernel_version": "5.15.0-91-generic",
      "arch": "amd64"
    }
  ]
}
```

**Agent Status:**
- `active`: Agent is registered and operational
- `inactive`: Agent manually deactivated
- `revoked`: Agent access revoked (cannot authenticate)

**Agent Health:**
- `healthy`: Last seen < 5 minutes ago
- `stale`: Last seen 5-15 minutes ago
- `inactive`: Last seen > 15 minutes ago OR status = revoked/inactive

---

### 8. Revoke Agent (User)

Revoke agent access permanently.

**Authentication:** Required (User)

**Request:**
```json
{
  "action": "revoke",
  "agent_id": "abc123xyz456"
}
```

**Response (200 OK):**
```json
{
  "success": true,
  "message": "agent revoked"
}
```

**Effects:**
- Sets agent status to `revoked`
- Clears refresh token
- Agent can no longer authenticate or perform actions

**Errors:**
- `400`: Missing agent_id
- `404`: Agent not found
- `403`: Agent doesn't belong to user

---

### 9. Agent Health (User)

Get detailed health status and latest metrics for an agent.

**Authentication:** Required (User)

**Request:**
```json
{
  "action": "health",
  "agent_id": "abc123xyz456"
}
```

**Response (200 OK):**
```json
{
  "agent_id": "abc123xyz456",
  "status": "active",
  "health": "healthy",
  "last_seen": "2024-01-15T10:30:45Z",
  "latest_metrics": {
    "cpu_percent": 45.2,
    "cpu_cores": 8,
    "memory_used_gb": 12.5,
    "memory_total_gb": 16.0,
    "memory_percent": 78.1,
    "disk_used_gb": 250.0,
    "disk_total_gb": 500.0,
    "disk_usage_percent": 50.0,
    "load_average": {
      "one_min": 2.15,
      "five_min": 1.98,
      "fifteen_min": 1.75
    },
    "network_stats": {
      "in_gb": 125.5,
      "out_gb": 89.3
    },
    "filesystems": [...]
  }
}
```

**Notes:**
- `latest_metrics` is `null` if no metrics have been reported yet
- Useful for dashboards and monitoring

---

## Static API Tokens

Static tokens are long-lived, user-owned API keys that can be shared by
one or more agents. Unlike the access/refresh pair, a static token:

- **Does not expire automatically** (unless an optional expiry is set).
- **Is not rotated** — the same value is used until the owner revokes it.
- **Can be shared across multiple agents** under the same user account.

All static tokens are prefixed with `nsk_` and contain 32 bytes of
cryptographically random entropy (hex-encoded). Only a SHA-256 hash of
the token is stored; the plaintext value is returned **exactly once**,
at creation time, and cannot be retrieved afterwards.

### Using a Static Token

Pass the token as a Bearer credential on any request:
```text
Authorization: Bearer nsk_<hex>
```

By default the request is authenticated as the **owning user**. To act
as a specific agent (e.g. for `ingest-metrics` or other per-agent
endpoints) also include an `X-Agent-ID` header identifying an agent
belonging to the same user:
```text
Authorization: Bearer nsk_<hex>
X-Agent-ID: <agent_id>
```

If `X-Agent-ID` does not reference an agent owned by the token's user,
the request is rejected with `403 invalid X-Agent-ID for static token`.

### 10. Create Static Token (User)

Issues a new static token. The plaintext token appears **only** in this
response — capture it immediately.

**Authentication:** Required (User).

**Request:**
```json
{
  "action": "create-static-token",
  "name": "ci-ingestion",
  "expires_in_days": 90
}
```

**Field Descriptions:**
- `name` (string, required, 1–120 chars): Human-readable label.
- `expires_in_days` (int, optional): One of `0`, `30`, `60`, `90`, `180`,
  `365`. `0` (the default) means the token never expires.

**Response (200 OK):**
```json
{
  "token": "nsk_7f2c4b1e3a9d8c6b5a4e3d2c1b0a9f8e7d6c5b4a3928170605040302010fedcb",
  "token_info": {
    "id": "tok_abc123",
    "name": "ci-ingestion",
    "token_prefix": "nsk_7f2c4b1e",
    "expires_at": "2026-07-22T10:45:00Z",
    "revoked": false,
    "revoked_at": null,
    "last_used_at": null,
    "created": "2026-04-22T10:45:00Z"
  }
}
```

**Errors:**
- `400`: `name required`
- `400`: `name too long (max 120)`
- `400`: `expires_in_days must be one of 0, 30, 60, 90, 180, 365`
- `401`: `authentication required`
- `403`: `users only` (static token / agent tokens cannot create new
  static tokens)

---

### 11. List Static Tokens (User)

Returns all static tokens owned by the authenticated user. Plaintext
values are never returned; only metadata.

**Authentication:** Required (User).

**Request:**
```json
{
  "action": "list-static-tokens"
}
```

**Response (200 OK):**
```json
{
  "tokens": [
    {
      "id": "tok_abc123",
      "name": "ci-ingestion",
      "token_prefix": "nsk_7f2c4b1e",
      "expires_at": "2026-07-22T10:45:00Z",
      "revoked": false,
      "last_used_at": "2026-04-22T12:03:11Z",
      "created": "2026-04-22T10:45:00Z"
    }
  ]
}
```

**Field Descriptions:**
- `token_prefix`: First 12 characters of the original token (for UI
  disambiguation only).
- `expires_at`: `null` if the token never expires.
- `last_used_at`: Updated on each successful authentication (best effort).

---

### 12. Revoke Static Token (User)

Permanently invalidates a static token. A revoked token cannot be
restored; issue a new one if needed.

**Authentication:** Required (User).

**Request:**
```json
{
  "action": "revoke-static-token",
  "token_id": "tok_abc123"
}
```

**Response (200 OK):**
```json
{
  "success": true,
  "message": "token revoked"
}
```

**Errors:**
- `400`: `token_id required`
- `401`: `authentication required`
- `403`: `not your token`
- `404`: `token not found`

---

## Investigations

Investigation endpoints for AI-powered diagnostics.

### Base Endpoints
```text
POST   /api/investigations  - Create investigation
GET    /api/investigations  - List or get investigation(s)
PATCH  /api/investigations  - Update investigation
```

### 1. Create Investigation

Initiate new investigation for an agent.

**Authentication:** Required (User or Agent)

**Request:**
```json
{
  "agent_id": "abc123xyz456",
  "issue": "High CPU usage on process 'mysqld', slow query responses",
  "priority": "high"
}
```

**Response (201 Created):**
```json
{
  "id": "inv_789",
  "user_id": "user_123",
  "agent_id": "abc123xyz456",
  "episode_id": "",
  "user_prompt": "High CPU usage on process 'mysqld', slow query responses",
  "priority": "high",
  "status": "pending",
  "resolution_plan": "",
  "initiated_at": "2024-01-15T10:45:00Z",
  "completed_at": null,
  "created_at": "2024-01-15T10:45:00Z",
  "updated_at": "2024-01-15T10:45:00Z",
  "metadata": {
    "initiated_by": "portal"
  },
  "inference_count": 0
}
```

**Field Descriptions:**
- `issue`: Must be at least 10 characters
- `priority`: `low`, `medium` (default), or `high`
- `episode_id`: Set on first TensorZero response
- `resolution_plan`: Populated when AI completes diagnosis
- `inference_count`: Number of AI inferences (from ClickHouse)

**Investigation Status:**
- `pending`: Created, waiting for agent to begin
- `in_progress`: Agent is executing diagnostic loop
- `completed`: AI has provided resolution plan
- `failed`: Investigation failed or timed out

**Errors:**
- `400`: Missing required fields or issue too short
- `403`: Agent doesn't belong to user
- `404`: Agent not found

---

### 2. List Investigations

Get all investigations for authenticated user.

**Authentication:** Required (User)

**Request:**
```text
GET /api/investigations
```

**Response (200 OK):**
```json
[
  {
    "id": "inv_789",
    "agent_id": "abc123xyz456",
    "user_prompt": "High CPU usage on process 'mysqld'",
    "priority": "high",
    "status": "completed",
    "initiated_at": "2024-01-15T10:45:00Z",
    "completed_at": "2024-01-15T10:52:30Z",
    "created_at": "2024-01-15T10:45:00Z",
    "inference_count": 5
  },
  {
    "id": "inv_788",
    "agent_id": "abc123xyz456",
    "user_prompt": "Disk I/O latency issues",
    "priority": "medium",
    "status": "in_progress",
    "initiated_at": "2024-01-15T09:30:00Z",
    "completed_at": null,
    "created_at": "2024-01-15T09:30:00Z",
    "inference_count": 3
  }
]
```

---

### 3. Get Investigation

Get detailed investigation with resolution plan and inferences.

**Authentication:** Required (User)

**Request:**
```text
GET /api/investigations?id=inv_789
```

**Response (200 OK):**
```json
{
  "id": "inv_789",
  "user_id": "user_123",
  "agent_id": "abc123xyz456",
  "episode_id": "ep_abc123",
  "user_prompt": "High CPU usage on process 'mysqld', slow query responses",
  "priority": "high",
  "status": "completed",
  "resolution_plan": "Root Cause: MySQL query cache disabled, causing repeated full table scans.\n\nResolution:\n1. Enable query cache: SET GLOBAL query_cache_size = 67108864;\n2. Add index on 'users.email' column\n3. Optimize slow queries identified in slow_query.log\n4. Consider upgrading to MySQL 8.0 for improved performance",
  "initiated_at": "2024-01-15T10:45:00Z",
  "completed_at": "2024-01-15T10:52:30Z",
  "created_at": "2024-01-15T10:45:00Z",
  "updated_at": "2024-01-15T10:52:30Z",
  "metadata": {
    "initiated_by": "portal",
    "inferences": [
      {
        "id": "inf_1",
        "function_name": "diagnose_and_heal",
        "variant_name": "v1",
        "timestamp": "2024-01-15T10:45:10Z",
        "processing_time_ms": 1250,
        "input": "...",
        "output": "..."
      }
    ]
  },
  "inference_count": 5
}
```

**Notes:**
- ClickHouse integration enriches response with inference data if configured
- `metadata.inferences` contains full AI conversation history
- Each inference includes input, output, model info, and timing

---

## Patch Management

Patch operation endpoints for system updates.

### Base Endpoints
```text
POST   /api/patches         - Create patch operation
GET    /api/patches         - List or get patch operation(s)
PATCH  /api/patches         - Update patch operation
POST   /api/patches/{id}/result - Upload execution results (Agent only)
GET    /api/scripts/{id}/validate - Validate script SHA256
```

### 1. Create Patch Operation

Initiate patch operation (dry-run or apply).

**Authentication:** Required (User)

**Request:**
```json
{
  "agent_id": "abc123xyz456",
  "mode": "dry-run"
}
```

**Response (201 Created):**
```json
{
  "id": "patch_001",
  "user_id": "user_123",
  "agent_id": "abc123xyz456",
  "mode": "dry-run",
  "status": "pending",
  "script_url": "/api/files/pbc_1234567890/script_abc/apt-update.sh",
  "script_sha256": "a3b5c7d9e1f3a5b7c9d1e3f5a7b9c1d3e5f7a9b1c3d5e7f9a1b3c5d7e9f1a3b5",
  "created_at": "2024-01-15T11:00:00Z",
  "updated_at": "2024-01-15T11:00:00Z"
}
```

**Field Descriptions:**
- `mode`: `dry-run` or `apply`
- `script_url`: Auto-populated based on agent's `platform_family`
- `script_sha256`: Used for integrity verification
- Status automatically set to `pending`

**Script Selection Logic:**
1. Determine agent's `platform_family` (debian, rhel, arch, or suse - only supported families)
2. Query `scripts` collection for matching platform
3. Prefer exact `os_version` match, fallback to generic family script
4. Auto-populate `script_id`, `script_url` in patch operation

**Package Exceptions:**
- Automatically populated from `package_exceptions` collection
- Active exceptions for the agent are injected as `exclusions` array
- Passed to script via `--exclude pkg1,pkg2` argument

**Errors:**
- `400`: Missing agent_id or invalid mode
- `403`: Agent doesn't belong to user
- `404`: No compatible script found for agent's platform (only debian, rhel, arch, suse supported)

---

### 2. List Patch Operations

Get all patch operations for authenticated user.

**Authentication:** Required (User)

**Request:**
```text
GET /api/patches
```

**Response (200 OK):**
```json
[
  {
    "id": "patch_001",
    "user_id": "user_123",
    "agent_id": "abc123xyz456",
    "mode": "dry-run",
    "status": "completed",
    "script_url": "/api/files/pbc_1234567890/script_abc/apt-update.sh",
    "created_at": "2024-01-15T11:00:00Z",
    "updated_at": "2024-01-15T11:02:30Z"
  }
]
```

---

### 3. Get Patch Operation

Get detailed patch operation with results.

**Authentication:** Required (User)

**Request:**
```text
GET /api/patches?id=patch_001
```

**Response (200 OK):**
```json
{
  "id": "patch_001",
  "user_id": "user_123",
  "agent_id": "abc123xyz456",
  "mode": "dry-run",
  "status": "completed",
  "script_url": "/api/files/pbc_1234567890/script_abc/apt-update.sh",
  "created_at": "2024-01-15T11:00:00Z",
  "updated_at": "2024-01-15T11:02:30Z"
}
```

---

### 4. Validate Script (Agent)

Agent validates script integrity before execution.

**Authentication:** Required (Agent)

**Request:**
```text
GET /api/scripts/{script_id}/validate
```

**Response (200 OK):**
```json
{
  "id": "script_abc",
  "sha256": "a3b5c7d9e1f3a5b7c9d1e3f5a7b9c1d3e5f7a9b1c3d5e7f9a1b3c5d7e9f1a3b5",
  "name": "apt-update.sh"
}
```

**Agent Workflow:**
1. Receive patch operation via realtime subscription
2. Call `/api/scripts/{id}/validate` to get expected SHA256
3. Download script from `script_url`
4. Calculate SHA256 hash of downloaded script
5. Compare with expected hash
6. If mismatch: ABORT and report error
7. If match: Execute script

**Notes:**
- Critical security feature preventing MITM attacks
- SHA256 automatically calculated when script is uploaded
- Agent must verify before every execution

---

### 5. Upload Patch Results (Agent)

Agent uploads execution results after patch completion.

**Authentication:** Required (Agent)

**Request (multipart/form-data):**
```text
POST /api/patches/{patch_id}/result
Content-Type: multipart/form-data

stdout_file: <file>
stderr_file: <file>
exit_code: 0
status: completed
```

**Form Fields:**
- `stdout_file`: File attachment with stdout
- `stderr_file`: File attachment with stderr
- `exit_code`: Integer exit code (0 = success)
- `status`: `completed` or `failed`

**Response (200 OK):**
```json
{
  "success": true,
  "message": "results uploaded"
}
```

**Notes:**
- Only the agent that owns the patch operation can upload results
- Files stored in PocketBase storage
- Accessible via `stdout_file` and `stderr_file` fields in patch operation record

**Errors:**
- `400`: Missing patch_id
- `403`: Patch operation doesn't belong to agent
- `404`: Patch operation not found

---

## Package Exceptions

Manage persistent package exclusions.

### Using PocketBase REST API

Package exceptions use standard PocketBase CRUD operations:

**Base URL:**
```text
/api/collections/package_exceptions/records
```

**Access Control:**
- User can only manage their own exceptions
- Agents can read exceptions for their patch operations

### Create Exception

**Request:**
```json
POST /api/collections/package_exceptions/records
Authorization: Bearer <user_token>

{
  "agent_id": "abc123xyz456",
  "package_name": "kernel",
  "reason": "Custom kernel build, do not update",
  "is_active": true,
  "expires_at": "2024-12-31T23:59:59Z"
}
```

### List Exceptions

**Request:**
```flowchart
GET /api/collections/package_exceptions/records?filter=agent_id='abc123xyz456'
Authorization: Bearer <user_token>
```

### Update Exception

**Request:**
```json
PATCH /api/collections/package_exceptions/records/{id}
Authorization: Bearer <user_token>

{
  "is_active": false
}
```

---

## PocketBase Standard Endpoints

NannyAPI leverages PocketBase's built-in REST API for collections not covered by custom endpoints.

### Authentication

**Login (Email/Password):**
```text
POST /api/collections/users/auth-with-password

{
  "identity": "user@example.com",
  "password": "SecurePass123!"
}
```

**OAuth2 (GitHub/Google):**
```text
GET /api/oauth2-redirect?provider=github
```

### Collections

All collections support standard CRUD with access rules:

**List:**
```text
GET /api/collections/{collection}/records?page=1&perPage=20
```

**Get:**
```text
GET /api/collections/{collection}/records/{id}
```

**Create:**
```text
POST /api/collections/{collection}/records
```

**Update:**
```text
PATCH /api/collections/{collection}/records/{id}
```

**Delete:**
```text
DELETE /api/collections/{collection}/records/{id}
```

### Filtering & Sorting

**Filter Syntax:**
```text
?filter=status='completed' && priority='high'
?filter=agent_id='abc123' && created>='2024-01-01'
```

**Sorting:**
```text
?sort=-created,+priority
```

**Expansion (Relations):**
```text
?expand=agent_id,user_id
```

---

## Real-time Subscriptions

PocketBase provides built-in SSE (Server-Sent Events) for real-time updates.

### Subscribe to Collection

```javascript
// Agent subscribes to patch operations
const eventSource = new EventSource(
  'http://localhost:8090/api/realtime',
  { withCredentials: true }
);

eventSource.addEventListener('PB_CONNECT', (e) => {
  const clientId = JSON.parse(e.data).clientId;
  
  // Subscribe to patch_operations for this agent
  fetch('http://localhost:8090/api/realtime', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Authorization': 'Bearer <agent_token>'
    },
    body: JSON.stringify({
      clientId: clientId,
      subscriptions: ['patch_operations']
    })
  });
});

eventSource.addEventListener('patch_operations', (e) => {
  const data = JSON.parse(e.data);
  console.log('New patch operation:', data.record);
});
```

### Subscription Events

- `create`: New record created
- `update`: Record updated
- `delete`: Record deleted

---

## Error Responses

All endpoints return standard error format:

**Error Response:**
```json
{
  "error": "Error message describing what went wrong"
}
```

**HTTP Status Codes:**
- `200 OK`: Successful request
- `201 Created`: Resource created successfully
- `400 Bad Request`: Invalid request parameters
- `401 Unauthorized`: Authentication required
- `403 Forbidden`: Access denied
- `404 Not Found`: Resource not found
- `500 Internal Server Error`: Server error

---

## Rate Limiting

PocketBase provides built-in rate limiting (configurable):
- Default: 120 requests/minute per IP
- Configurable via `--throttleThreshold` flag

---

## File Downloads

Download files (scripts, patch outputs) via PocketBase file API:

**Syntax:**
```text
GET /api/files/{collection}/{record_id}/{filename}
Authorization: Bearer <token>
```

**Example:**
```text
GET /api/files/pbc_1234567890/script_abc/apt-update.sh
Authorization: Bearer <agent_token>
```

**Notes:**
- Access control enforced based on collection rules
- Scripts are publicly readable by authenticated users/agents
- Patch outputs only readable by operation owner

---

## Pagination

All list endpoints support pagination:

**Query Parameters:**
- `page`: Page number (default: 1)
- `perPage`: Items per page (default: 30, max: 500)

**Response Headers:**
```text
X-Pagination-Page: 1
X-Pagination-Per-Page: 30
X-Pagination-Total-Items: 150
X-Pagination-Total-Pages: 5
```

---

## Complete Example: Agent Lifecycle

### 1. Registration
```bash
# Agent requests device code
curl -X POST http://localhost:8090/api/agent \
  -H "Content-Type: application/json" \
  -d '{"action": "device-auth-start"}'

# Response: {"device_code": "...", "user_code": "ABCD1234", ...}

# User authorizes (browser)
curl -X POST http://localhost:8090/api/agent \
  -H "Authorization: Bearer <user_token>" \
  -H "Content-Type: application/json" \
  -d '{"action": "authorize", "user_code": "ABCD1234"}'

# Agent registers
curl -X POST http://localhost:8090/api/agent \
  -H "Content-Type: application/json" \
  -d '{
    "action": "register",
    "device_code": "...",
    "hostname": "server-01",
    "os_type": "linux",
    "platform_family": "debian",
    ...
  }'

# Response: {"access_token": "...", "refresh_token": "...", "agent_id": "..."}
```

### 2. Metrics Reporting
```bash
curl -X POST http://localhost:8090/api/agent \
  -H "Authorization: Bearer <agent_token>" \
  -H "Content-Type: application/json" \
  -d '{
    "action": "ingest-metrics",
    "system_metrics": {
      "cpu_percent": 45.2,
      "cpu_cores": 8,
      ...
    }
  }'
```

### 3. Patch Execution
```bash
# Agent receives patch operation via realtime
# Agent validates script
curl http://localhost:8090/api/scripts/script_abc/validate \
  -H "Authorization: Bearer <agent_token>"

# Agent downloads and verifies script
curl http://localhost:8090/api/files/.../apt-update.sh \
  -H "Authorization: Bearer <agent_token>" \
  -o script.sh

# Verify SHA256
sha256sum script.sh

# Execute
./script.sh --dry-run --exclude kernel,linux-image

# Upload results
curl -X POST http://localhost:8090/api/patches/patch_001/result \
  -H "Authorization: Bearer <agent_token>" \
  -F "stdout_file=@stdout.log" \
  -F "stderr_file=@stderr.log" \
  -F "exit_code=0" \
  -F "status=completed"

---

## Operational Observability

NannyAPI persists a server-side audit trail of realtime events and actively
sweeps operations that get stuck in non-terminal states. This addresses a
class of failures where PocketBase's built-in realtime (SSE) publish is
fire-and-forget: each event is handed to every subscriber's buffered
channel in a goroutine with no persistence, no retry, and no delivery
ACK. A briefly disconnected agent silently loses events with no trace on
the server.

### Realtime Message Outbox

Every create/update on `patch_operations`, `reboot_operations`, and
`investigations` triggers a row in the `realtime_messages` collection
and a structured log line (`app.Logger().Info("realtime event", ...)`).

**`realtime_messages` fields:**

| Field             | Description                                                           |
| ----------------- | --------------------------------------------------------------------- |
| `resource_type`   | `patch_operations`, `reboot_operations`, or `investigations`.         |
| `resource_id`     | ID of the row that changed.                                           |
| `action`          | `created` or `updated`.                                               |
| `resource_status` | Value of the operation's `status` field at write time.                |
| `agent_id`        | Target agent when resolvable.                                         |
| `user_id`         | Owning user when resolvable.                                          |
| `delivery_status` | `logged` on success, `reaper_failed` when emitted by the reaper.      |
| `error`           | Reason string (max 500 chars) if the event represents a failure.      |
| `payload`         | JSON snapshot of the emitting record (max 100KB).                     |
| `created`         | Write timestamp (indexed).                                            |

Typical operational queries:

- "Did the UI receive the patch-complete event?" → filter by
  `resource_id` and check for an `action=updated` with
  `resource_status=completed`.
- "Which operations the reaper had to fail?" → filter by
  `delivery_status=reaper_failed`.

### Stuck Operation Reaper

A background cron scans the three tracked collections and marks any
record stuck in a non-terminal status beyond a configurable timeout as
failed, then emits a synthetic `reaper_failed` outbox row so UIs and
agents stop spinning forever.

**Terminal-failure status per resource:**

| Collection           | Stuck statuses                    | Set to     | Error field     |
| -------------------- | --------------------------------- | ---------- | --------------- |
| `patch_operations`   | `pending`, `running`              | `failed`   | `error_msg`     |
| `reboot_operations`  | `pending`, `sent`, `rebooting`    | `timeout`  | `error_message` |
| `investigations`     | `pending`, `in_progress`          | `failed`   | `error`         |

**Environment variables** (both optional; shown with defaults):

| Variable                          | Default | Meaning                                                |
| --------------------------------- | ------- | ------------------------------------------------------ |
| `STUCK_OP_TIMEOUT_SECONDS`        | `3600`  | How long an operation may sit in a non-terminal state. |
| `STUCK_OP_SCAN_INTERVAL_SECONDS`  | `300`   | How often the reaper scans for stuck operations.       |

The reaper starts automatically on server boot (10-second warm-up
delay). No manual endpoint is required or exposed.
