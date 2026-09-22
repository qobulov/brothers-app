# Auth Implementation Plan

Generated from: provided auth screens and database tables
Date: 2026-09-22

## Agreed flow

- Registration: language selection -> account details -> open Telegram bot deep link -> press Start -> receive OTP in the bot -> verify OTP in the app -> authenticated session.
- Login: username or `+998` phone number + password.
- Password recovery: phone submission -> open a reset-specific Telegram deep link -> press Start -> receive OTP in the bot -> short-lived reset token -> new password.
- Phone change: authenticated user submits the new phone -> receives a phone-change Telegram deep link -> presses Start -> receives OTP -> confirms the change.
- OTP exists only for `registration`, `password_reset`, and `phone_change`; password login never requires OTP.
- Session model: one active session per user, with a short-lived access JWT and an opaque rotating refresh token stored only as a hash in `user_sessions`. A new login atomically revokes the previous active session.
- Telegram updates are consumed by a singleton `getUpdates` long-poll worker; the public API exposes no Telegram webhook route and no challenge-status polling route.
- Product decision: `/start` is only the delivery trigger, not phone ownership verification. Telegram Bot API does not expose a user's phone on Start. Treat `phone` as a unique account identifier and never label it phone-verified.
- Avatar upload is a separate media concern; auth receives only an optional `avatar_url` returned by the upload service.
- Every JSON API response uses one envelope: `success`, `code`, `slug`, `message`, `data`, and `meta`. HTTP status codes remain semantically correct; `code: 0` and `slug: "ok"` are reserved for successful operations.

## Standard response contract

Successful response:

```json
{
  "success": true,
  "code": 0,
  "slug": "ok",
  "message": "Запрос успешно обработан",
  "data": {},
  "meta": {
    "timestamp": "2026-09-22T12:55:03Z",
    "request_id": "aaddf89a9a081829a856613eff19682b",
    "api_version": "v1",
    "service": "brothers_app",
    "duration": "2.22896ms"
  }
}
```

Error response keeps the same shape:

```json
{
  "success": false,
  "code": 1404,
  "slug": "invalid_or_expired_otp",
  "message": "Неверный или просроченный код",
  "data": null,
  "meta": {
    "timestamp": "2026-09-22T12:55:03Z",
    "request_id": "aaddf89a9a081829a856613eff19682b",
    "api_version": "v1",
    "service": "brothers_app",
    "duration": "2.22896ms"
  }
}
```

Contract rules:

- `success` is derived from the result and cannot disagree with the HTTP status.
- `code` is `0` on success; failures use a documented stable application code, never a PostgreSQL/GORM/Telegram provider code.
- `slug` is a stable, lowercase machine-readable identifier such as `ok`, `invalid_credentials`, `bot_not_started`, or `rate_limit_exceeded`.
- `message` is user-facing and localized from `Accept-Language` before login, then from the authenticated user's language when available; fallback is Uzbek or the configured default.
- `data` always exists. It contains the endpoint payload on success and is `null` on ordinary errors; validation errors may use a documented `{"fields": ...}` object.
- `meta.timestamp` is server UTC in RFC3339, `request_id` is generated/propagated by middleware and also returned in `X-Request-ID`, `api_version` and `service` come from validated config, and `duration` covers the complete HTTP request.
- Internal errors, SQL messages, Telegram errors, secrets, tokens, OTP values, and stack traces never appear in `message` or `data`.
- All application JSON endpoints use this contract. Swagger/static/binary responses may keep their native content type. Telegram Bot API traffic is internal worker-to-provider traffic and is not part of the public response contract.

Every endpoint that starts one of the three OTP flows returns the deep link directly in `data`:

```json
{
  "success": true,
  "code": 0,
  "slug": "ok",
  "message": "Запрос успешно обработан",
  "data": {
    "telegram_deep_link": "https://t.me/brothers_auth_bot?start=<one-time-token>",
    "expires_at": "2026-09-22T13:05:03Z",
    "resend_in": 60
  },
  "meta": {
    "timestamp": "2026-09-22T12:55:03Z",
    "request_id": "aaddf89a9a081829a856613eff19682b",
    "api_version": "v1",
    "service": "brothers_app",
    "duration": "2.22896ms"
  }
}
```

Challenge database IDs are internal-only and never appear in an HTTP request or response. Registration and password-reset verification resolve the single active challenge by normalized phone plus endpoint-implied purpose; phone-change resolves it by authenticated user plus normalized new phone. Starting a replacement flow atomically supersedes the previous active challenge, so lookup is never based on an ambiguous "latest row" query.

## Smartup auth reference decisions

The local `/Users/n/github/smartup/smartup_auth_service` repository was reviewed on branch `refactor/readability-batch-2026-09-14` at commit `fa8f13d` (2026-09-21). Reuse the proven behavior, but port it to Fiber and the flat response contract above.

- **Reuse the metadata and error-catalog idea** from `server/http/response/response.go`: centralized code/slug/HTTP mappings and request metadata. Do not copy its nested error shape (`error.code`, `error.slug`, ...); Brothers requires the same flat six top-level fields for success and failure.
- **Reuse request tracing behavior** from `server/http/middleware/request_id.go`: start timing before handlers, generate IDs with `crypto/rand`, store them in request context, and echo `X-Request-ID`. Add length/character validation before trusting an incoming ID.
- **Reuse token primitives** from `pkg/token/token.go`: pinned HS256 validation with issuer/audience/expiry/issued-at checks, 32 random bytes for opaque refresh tokens, and hash-only refresh storage. Extend access claims with Brothers `sid` and explicit token `type`.
- **Reuse atomic refresh rotation** from `storage/postgres/refresh_token.go`, adapted to Brothers' single-session rule: conditionally replace the hash on the one active session row so concurrent refresh calls yield exactly one winner. Do not add session-family or descendant-token machinery.
- **Reuse the password transaction boundary** from `storage/postgres/password.go`: OTP consumption, password update, session revocation, replacement session creation, and audit insertion commit or roll back together.
- **Reuse OTP generation only** from `pkg/otp/otp.go`: six numeric digits from `crypto/rand`. For Brothers, hash OTPs with HMAC-SHA-256 plus a server-side OTP pepper and compare in constant time; plain SHA-256 is too weak for a six-digit value after a database leak.
- **Reuse versioned SQL migration organization** from `migrations/*.up.sql` and `*.down.sql`; do not retain GORM `AutoMigrate` as the production schema mechanism.
- **Do not reuse the current Telegram feature as auth delivery**: `pkg/telegram/client.go` only sends best-effort operational alerts to one configured chat and has no `/start`, user binding, or delivery state. Reuse only its bounded HTTP client/test style when building the new bot adapter and long-poll worker.
- **Do not reuse the in-memory rate limiter for production**: `server/http/middleware/ratelimit.go` keeps an unbounded per-IP map and is process-local. Use Redis-backed, TTL-bound limits for login, bot start, OTP verify/resend, refresh, and recovery.
- **Port the concurrency tests** from refresh rotation, OTP consumption, and password reset: exactly one concurrent request succeeds, the losing request receives a stable auth error, and failed transactions leave the old OTP/password/sessions usable exactly as specified.
- **Fix OTP expiry and revival gaps while porting**: Smartup validates expiry before a separate consume update, and an older unconsumed code can become current again after the newest code is consumed. Brothers must invalidate previous active challenges on issuance and atomically consume only a matching, unconsumed, unexpired challenge.
- **Add lifecycle cleanup**: schedule bounded batch deletion/anonymization for expired challenges, processed Telegram update IDs, and long-expired/revoked session history; Smartup defines refresh cleanup but has no complete production retention flow.

## Phase 1 — Schema and migrations

- [ ] **Adopt versioned migrations**: add `golang-migrate` and numbered `up/down` SQL files; stop using GORM `AutoMigrate` outside tests. The first migration must reconcile the current `users(email, password, name)` model with the target phone/username model without silently dropping existing data. _Modifies: database startup and deployment flow._
- [ ] **Finalize `users` authentication fields**: make `password_hash` required for password-based accounts, normalize `phone` to E.164 before persistence, and constrain `language` to supported values (`uz`, `ru`, `en`). Do not add `phone_verified_at`: this flow verifies Telegram control, not phone ownership. Decide explicitly whether soft-deleted usernames/phones may be reused before defining unique indexes. _Modifies: `users`._
- [ ] **Harden `user_sessions` for one active session**: add a unique index on `refresh_token_hash`, indexes on `(user_id, revoked_at)` and `expires_at`, a real foreign key to `users`, and a partial unique index on `user_id WHERE revoked_at IS NULL`. Retain `device_id` and `device_name`; do not add session-family/parent/descendant columns. Every session-creating transaction must revoke the current active row before inserting its replacement. _Modifies: `user_sessions`._
- [ ] **Choose OTP storage later**: the initial schema intentionally has no OTP table while the auth server/storage decision is pending. Telegram remains only a delivery channel; do not create a user Telegram account/binding table or request contacts. Add a dedicated Redis/SQL OTP store in the auth-server migration once the delivery and retention requirements are finalized.
- [ ] **Make Telegram long-poll processing idempotent**: persist the last committed Telegram `update_id`/offset plus a unique processed-update constraint so redelivery after a worker restart cannot send multiple OTPs or consume a challenge twice. Run exactly one poller per bot token, enforced by deployment topology or a distributed lease. _New persistence constraint for worker reliability._
- [ ] **Review group constraints alongside the migration**: add explicit foreign keys for group ownership/membership/invitations, ensure invitation `token_hash` is unique, and make `group_members.deleted_at` nullable so active memberships can exist. _Modifies: group-related tables; no auth behavior yet._

Suggested migration set:

1. `000001_reconcile_users_auth.up.sql` / `.down.sql`
2. `000002_create_user_sessions.up.sql` / `.down.sql`
3. `000003_create_otp_storage.up.sql` / `.down.sql` (deferred)
4. `000004_create_telegram_update_offsets.up.sql` / `.down.sql`
5. `000005_harden_group_constraints.up.sql` / `.down.sql`

Each `down` migration must reverse only its matching `up` migration. Existing user data must be backfilled and validated before adding `NOT NULL` or unique constraints; destructive column removal belongs in a later, explicitly approved cleanup migration.

## Phase 2 — Auth domain foundation

- [ ] **Replace the current user entity/DTO contract**: model UUID, phone, username, password hash, names, avatar URL, language, active/verified state, login timestamp, and timestamps; never serialize `password_hash`. _Modifies: `internal/entities/user.go`, user DTOs and mappers._
- [ ] **Build the standard response envelope**: replace ad-hoc `fiber.Map`, `ErrorResponse`, and direct JSON responses with typed `Envelope[T]` and `Meta` structs plus centralized success/error writers. Add request-context middleware that starts a timer, validates or generates `request_id`, resolves locale, and populates metadata exactly once. _Modifies: `pkg/responses`, Fiber middleware, and every handler._
- [ ] **Introduce auth-specific request/response DTOs**: define registration, login, registration OTP verify/resend, refresh, forgot-password verify/reset/resend, and phone-change request/confirm/resend contracts with boundary validation and normalized phone/username values. Every OTP-start response includes only `telegram_deep_link`, `expires_at`, and `resend_in`; public DTOs contain no challenge identifier. _New auth DTOs; reuses standard response helpers after extending them._
- [ ] **Extract small auth dependencies**: inject a password hasher, access-token signer/verifier, refresh-token generator, OTP generator, Telegram bot sender, repository interfaces, and a clock. Keep interfaces in the consuming use-case package so unit tests can use fakes. _New auth use-case dependencies._
- [ ] **Centralize auth errors**: add stable domain errors such as invalid credentials, invalid/expired OTP, too many attempts, inactive account, username/phone conflict, invalid refresh token, and expired reset token; assign each a stable numeric `code`, `slug`, localized message key, and HTTP status without exposing DB, crypto, or Telegram errors. _Modifies: `pkg/apperror` and response mapping._

## Phase 3 — Registration and OTP

- [ ] **Implement registration start**: `POST /api/v1/auth/register` validates names, username, E.164 phone, password policy, language, and optional avatar URL; hashes the password, creates or safely resumes an inactive unverified user, supersedes any previous active `registration` challenge for that phone, and creates a replacement with a random one-time bot-start token. Return only `telegram_deep_link`, `expires_at`, and `resend_in`; do not generate/send OTP before Telegram verification. The start parameter must be base64url-safe and at most 64 characters. _Creates: auth handler/use case/repository methods; depends on schema and auth foundation._
- [ ] **Run a Telegram `/start` long-poll worker and deliver OTP**: use Bot API `getUpdates` with a bounded long-poll timeout, accept only private-chat `/start <token>`, and hash/resolve the token. Store only the challenge's private delivery chat ID and immediately send OTP. Do not request or process Telegram contacts or persist Telegram account bindings. Commit the update offset only after durable processing; duplicate updates must be harmless. _New internal worker and Telegram adapter; no public webhook endpoint._
- [ ] **Implement registration OTP verification**: `POST /api/v1/auth/otp/verify` accepts normalized `phone` and OTP. Resolve the sole active `registration` challenge for that phone and require `started_at`, a delivery chat ID, and successful Telegram delivery; then atomically consume only the unconsumed, non-superseded, unexpired row within the attempt limit. In the same transaction activate the user, revoke any existing active session, and create the sole active session; exactly one concurrent verifier may succeed. Do not mark the submitted phone as verified. _Depends on OTP delivery and session issuance._
- [ ] **Implement flow-specific OTP resend**: `POST /api/v1/auth/register/resend` and `POST /api/v1/auth/password/resend` accept normalized phone; authenticated `POST /api/v1/me/phone-change/resend` accepts normalized new phone. Each endpoint implies one fixed purpose and resolves only the sole active challenge for its subject. One transaction enforces cooldown, invalidates the previous code generation, and increments resend count so parallel requests cannot issue two valid codes. Send only to the bound private `chat_id`; before Start return `bot_not_started`. _Covers cooldown, maximum resends, expiry, and consumption without exposing internal IDs._

## Phase 4 — Login and session lifecycle

- [ ] **Implement username-or-phone login**: `POST /api/v1/auth/login` resolves a normalized identifier, performs a constant-behavior credential check, and rejects inactive/unverified users. Lock the user row with `SELECT ... FOR UPDATE`; then update `last_login_at`, revoke the previous active session, and create the user's sole active session in one transaction. Return the new token pair plus safe user data. _Replaces current email login; serializes concurrent logins and requires no login OTP._
- [ ] **Issue secure token pairs**: use a short-lived access JWT (recommended 15 minutes) with `sub`, `sid`, `iss`, `aud`, `iat`, `exp`, and token type claims; generate the refresh token with `crypto/rand`, persist only its SHA-256 hash, and return plaintext once. _Creates: token service and session repository._
- [ ] **Implement logout**: authenticated `POST /api/v1/auth/logout` revokes the current `sid`. No `logout-all` endpoint is needed because a user can have only one active session. _Modifies: private routes and middleware context._
- [ ] **Replace JWT middleware**: safely parse the `Bearer` scheme, pin the signing algorithm, validate issuer/audience/expiry/token type, and require valid UUID `sub` and `sid`. Confirm the referenced session is active on every protected request (directly or through short TTL Redis cache invalidated on logout/reset) so logout and password reset invalidate access immediately; otherwise document that access survives until its 15-minute expiry. Avoid unchecked slicing and claim assertions in the current middleware. _Modifies: `pkg/middleware/jwt.go`._

## Phase 5 — Password recovery

- [ ] **Implement forgot-password request**: `POST /api/v1/auth/password/forgot` accepts a phone number, supersedes its previous active password-reset flow, and returns only a reset-specific `telegram_deep_link`, `expires_at`, and `resend_in`. Opening it and pressing Start binds that exact internal OTP challenge and sends the code to that private chat. Keep response shape/timing generic for unknown phones, using a non-actionable decoy link if necessary to prevent account enumeration.
- [ ] **Verify reset OTP**: `POST /api/v1/auth/password/verify` accepts normalized `phone` and OTP, resolves and atomically consumes the sole active `password_reset` challenge, and returns a single-use, short-lived reset token scoped only to password replacement. It must not return a normal access token. _Depends on auth challenges._
- [ ] **Reset the password**: `POST /api/v1/auth/password/reset` validates the reset token and password confirmation/policy, updates `password_hash` in a transaction, consumes the reset grant, and revokes the user's sole active session if present. _Completes the reset-password screen._

## Phase 6 — Profile and UI support

- [ ] **Implement current-user endpoint**: `GET /api/v1/me` returns only safe profile fields and verified/auth state; `PATCH /api/v1/me` supports language, names, and avatar URL changes with uniqueness/format validation where applicable. _Modifies the existing `/me` handler._
- [ ] **Implement phone-change request**: authenticated `POST /api/v1/me/phone-change/request` validates that the normalized new phone is available, supersedes the user's previous active `phone_change` challenge, and returns only `telegram_deep_link`, `expires_at`, and `resend_in`. On Start, the Telegram worker stores the private delivery chat ID and immediately sends OTP; it does not request contact access. _Adds the third and final OTP purpose._
- [ ] **Confirm the phone change**: authenticated `POST /api/v1/me/phone-change/confirm` accepts normalized `new_phone` and OTP, resolves the sole active `phone_change` challenge for the access-token user and that phone, and atomically consumes it while updating `users.phone` after rechecking uniqueness. The current session remains active because the authenticated user ID does not change. The new phone remains an unverified account identifier. _Completes phone-number change._
- [ ] **Keep language selection client-first**: persist the pre-login selection locally and submit it during registration; authenticated changes go through `PATCH /me`. No standalone public language endpoint is required. _Reuses the registration/profile contracts._
- [ ] **Align auth-screen copy with Telegram delivery**: replace every `SMS`, "sent to phone", and `Resend SMS` label in registration/recovery/phone-change OTP screens with Telegram-specific instructions: open the bot, press Start, receive the code in Telegram, and resend the Telegram code. Never show a contact-sharing step. _Prevents the UI from promising SMS delivery._

## Phase 7 — Security and operational controls

- [ ] **Apply abuse protection**: rate-limit registration, login, OTP request/verify, refresh, and password recovery by normalized identity plus IP; cap request body size and OTP attempts; add audit events without logging passwords, OTPs, access tokens, refresh tokens, or full phone numbers. _New middleware/policy._
- [ ] **Move all auth configuration into validated config**: require non-default JWT secret, issuer/audience, access/refresh/reset/OTP TTLs, bcrypt cost, resend cooldown, attempt limits, Telegram bot token/username, Bot API request timeout, long-poll timeout, and worker lease settings at startup. Do not configure a webhook secret or public webhook URL. _Modifies: `pkg/config`; removes direct `os.Getenv` reads from auth code._
- [ ] **Define Telegram delivery behavior**: call Bot API `sendMessage` with a timeout and idempotent delivery record; mark `otp_sent` only after Telegram accepts the message, and map blocked/deactivated chat or provider failures to a retryable challenge state without exposing provider details. _New Telegram adapter._
- [ ] **Operate the long-poll worker safely**: ensure no webhook is configured for this bot token, allow only one active poller through a dedicated worker replica or distributed lease, persist offsets, use bounded retries/backoff, expose health/lag metrics, and shut down without advancing the offset past unfinished work. _New background worker lifecycle._
- [ ] **Run retention cleanup**: periodically delete in bounded batches after configured retention windows for expired/consumed auth challenges, processed Telegram update IDs, and expired/revoked sessions; expose cleanup counts/errors as operational metrics. _New background maintenance job._
- [ ] **Update Swagger/OpenAPI**: document request/response schemas, auth requirements, error codes, rate-limit responses, and examples for every auth endpoint. _Modifies generated API docs._

## Phase 8 — Verification

- [ ] **Unit-test each use case**: table-driven tests cover normalization, duplicate phone/username, password policy, invalid/expired/replayed start tokens, duplicate/redelivered Telegram updates, active-challenge supersession, wrong/expired/consumed OTP, strict purpose separation, attempt and resend limits, inactive users, login ambiguity, single-session replacement, concurrent refresh, reset-token scope, and session revocation. Assert that `/start` causes immediate OTP delivery and that no contact update is required. Inject a fake clock and fake OTP sender; never sleep in tests. _Creates tests alongside auth use-case files._
- [ ] **Add repository integration tests**: behind the `integration` build tag, verify one-active-challenge constraints, transactional supersession/OTP consumption, concurrent verify/refresh attempts, row locking, and revocation behavior against PostgreSQL. _Creates isolated database tests._
- [ ] **Add HTTP contract tests**: use Fiber test requests to assert the exact six top-level fields on every success and error path, `code: 0` only on success, stable error codes/slugs, localized messages, non-empty request metadata, matching `X-Request-ID`, generic forgot-password responses, authorization boundaries, malformed bearer headers, absence of sensitive fields, and absence of any public challenge identifier. _Creates handler and middleware tests._
- [ ] **Run release gates**: `go test ./...`, `go test -race ./...`, `go vet ./...`, static analysis, migration up/down rehearsal on a disposable database, and a smoke flow covering register -> OTP -> me -> refresh -> logout and forgot -> OTP -> reset -> old-session rejection. _Final verification._

## Suggested endpoint contract

| Method | Path | Auth | Purpose |
| --- | --- | --- | --- |
| POST | `/api/v1/auth/register` | Public | Save pending account and return Telegram deep link |
| POST | `/api/v1/auth/otp/verify` | Public | Verify registration OTP by phone and issue token pair |
| POST | `/api/v1/auth/register/resend` | Public | Resend registration OTP by phone |
| POST | `/api/v1/auth/login` | Public | Login by username or phone |
| POST | `/api/v1/auth/refresh` | Refresh token | Rotate tokens on the sole active session |
| POST | `/api/v1/auth/logout` | Access token | Revoke current session |
| POST | `/api/v1/auth/password/forgot` | Public | Start reset OTP and return Telegram deep link |
| POST | `/api/v1/auth/password/resend` | Public | Resend password-reset OTP by phone |
| POST | `/api/v1/auth/password/verify` | Public | Exchange phone plus reset OTP for reset-only token |
| POST | `/api/v1/auth/password/reset` | Reset token | Set new password and revoke the active session |
| GET | `/api/v1/me` | Access token | Current profile |
| PATCH | `/api/v1/me` | Access token | Update profile/language/avatar URL |
| POST | `/api/v1/me/phone-change/request` | Access token | Start phone-change OTP and return Telegram deep link |
| POST | `/api/v1/me/phone-change/resend` | Access token | Resend phone-change OTP for the new phone |
| POST | `/api/v1/me/phone-change/confirm` | Access token | Verify new phone plus OTP and apply it |

## Recommended build order

1. Versioned migrations and final schema decisions.
2. Auth domain contracts, validation, repositories, and injected crypto/clock/Telegram ports.
3. Registration -> Telegram long-poll `/start` -> immediate OTP delivery -> verification as the first end-to-end vertical slice.
4. Login + token pair issuance + hardened middleware.
5. Single-row refresh rotation + logout/session revocation.
6. Forgot-password + reset-password, then phone-change OTP flow.
7. Profile/UI support, Swagger, security controls, and full verification suite.
