# Auth API Tasks: Telegram OTP

Updated: 2026-09-23

## Final registration flow

1. Client calls `POST /api/v1/auth/otp/send` with the phone and
   `purpose="registration"`.
2. Backend checks that the phone is not registered, stores the Telegram Start flow in
   Redis, and returns a one-time bot deep link with `ttl` and `resend_in`.
3. User opens the link and presses Start. The bot generates a six-digit OTP, stores
   only its hash in Redis, and sends the code to the Telegram chat.
4. Client calls `POST /api/v1/auth/register` with the profile, password, phone, and
   `otp_code`.
5. Backend verifies and consumes the registration OTP, inserts an active user using
   SQLC + pgx, creates a session, and returns the user and token pair.

There is no pending user, `CreatePendingUser`, `/auth/register/verify`, generic
`/auth/otp/verify`, registration resend endpoint, OTP database table, GORM model,
challenge, or E-IMZO flow.

Login remains password-based through `POST /api/v1/auth/login`; `login` accepts a
username or normalized phone number.

## Public contracts

### Send registration OTP

`POST /api/v1/auth/otp/send`

```json
{
  "phone": "+998901234567",
  "purpose": "registration"
}
```

The response contains `delivery`, an optional `telegram_deep_link`, `ttl`,
`expires_at`, and `resend_in`. It never returns the OTP, its hash, Redis keys, a
Telegram chat ID, or database identifiers.

Calling the same endpoint after the Telegram chat is bound sends a replacement OTP.
Cooldown violations return `429 Too Many Requests`. A registered phone returns a
conflict.

### Register and verify OTP

`POST /api/v1/auth/register`

```json
{
  "avatar_url": "https://example.com/avatar.jpg",
  "first_name": "Qobul",
  "language": "uz",
  "last_name": "Qobulov",
  "password": "strong-password",
  "phone": "+998901234567",
  "username": "qobulov",
  "otp_code": "482910"
}
```

The endpoint validates the request and the registration-purpose OTP, consumes the OTP
once, then directly creates an active user. Request DTOs do not accept IDs, active
flags, roles, or database-managed timestamps.

Successful `data` shape:

```json
{
  "tokens": {
    "access_token": "<jwt>",
    "access_expires_at": "2026-09-23T10:15:00Z",
    "refresh_token": "<opaque-token>",
    "refresh_expires_at": "2026-10-23T10:00:00Z"
  },
  "user": {
    "id": "<uuid>",
    "full_name": "Qobul Qobulov",
    "phone": "+998901234567",
    "role": "user"
  }
}
```

### Login

`POST /api/v1/auth/login`

```json
{
  "login": "qobulov",
  "password": "strong-password"
}
```

## Storage and security rules

- PostgreSQL access uses SQLC-generated queries with pgx; GORM is not allowed.
- Registration uses one direct `CreateAuthUser` query with `is_active=true`.
- OTP, Telegram Start tokens, cooldowns, and attempt counters live only in Redis.
- OTP values are generated with `crypto/rand`, stored as keyed hashes, compared in
  constant time, expire automatically, and are consumed once.
- Unique phone or username conflicts are enforced by PostgreSQL and mapped to a stable
  application conflict error.
- Passwords are stored only as bcrypt hashes. Refresh tokens are random and stored only
  as hashes; access tokens are short-lived signed JWTs.
- Secrets and provider/database errors never appear in HTTP responses or Swagger.

## Verification checklist

- [x] `POST /auth/otp/send` returns the Telegram delivery state.
- [x] `POST /auth/register` requires `otp_code` and returns nested tokens/user data.
- [x] Registration inserts an active user directly through SQLC.
- [x] Pending-user queries and activation code are removed.
- [x] Redundant verification/resend routes are removed.
- [x] Swagger describes the implemented flow.
- [x] Run the complete Go test suite and `go vet ./...` after every auth change.
- [ ] Smoke-test send OTP, Telegram Start, register, login, refresh, and logout locally.
