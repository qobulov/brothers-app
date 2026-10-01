# Personal Debts — Design

Stage 1 of the debts feature. Stage 2 adds "Mark as debt" on orders, which
creates a debt from an order side and changes the order flow.

## Goal

Let a user keep a private record of money they owe and money owed to them:
create a debt, record partial repayments, close it, and see totals per
currency.

Screens covered: Debts list (summary, filters, search), Debt Details (detail
and repayment history), Add Debt.

## Decisions

| Topic | Decision |
|---|---|
| Ownership | A debt belongs to one user. Nobody else can see it: not group members, owners or admins. Debts are not tied to a group. |
| Currencies | `USD` or `UZS`, fixed at creation. Whole numbers only. |
| Totals | Per currency and per direction, never converted. No exchange rate. |
| Repayments | Each repayment is an immutable record; the History tab lists them. |
| Remaining amount | Stored on the debt and decreased under a row lock in the same transaction that records the repayment. |
| Reaching zero | A repayment that brings the remaining amount to 0 completes the debt. |
| Mark as Completed | Closes an active debt at any time. The remaining amount is left unchanged; no repayment is invented. |
| Reopen | Not possible. |
| Delete | Allowed at any time, even with repayments. Soft delete. |
| Edit | Not supported (not in the design). |
| Order debts | Out of scope. `source_order_id` exists for stage 2 and is always empty now. |
| Architecture | New `internal/debt` package with `Handler` and `Service`, hand-written SQL, as in `internal/group` and `internal/order`. |

## Data model

### Migration `17_debts.up.sql`

```sql
CREATE TABLE IF NOT EXISTS debts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_user_id uuid NOT NULL REFERENCES users(id),
    direction varchar(16) NOT NULL,
    person_name varchar(100) NOT NULL,
    person_phone varchar(20),
    currency varchar(3) NOT NULL,
    original_amount bigint NOT NULL,
    remaining_amount bigint NOT NULL,
    status varchar(16) NOT NULL DEFAULT 'active',
    source_order_id uuid REFERENCES orders(id),
    completed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz,
    CONSTRAINT debts_direction_check CHECK (direction IN ('they_owe_me', 'i_owe')),
    CONSTRAINT debts_currency_check CHECK (currency IN ('USD', 'UZS')),
    CONSTRAINT debts_status_check CHECK (status IN ('active', 'completed')),
    CONSTRAINT debts_amounts_check CHECK (original_amount > 0 AND remaining_amount BETWEEN 0 AND original_amount)
);

CREATE INDEX IF NOT EXISTS debts_owner_created_idx
    ON debts (owner_user_id, created_at DESC, id DESC)
    WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS debt_repayments (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    debt_id uuid NOT NULL REFERENCES debts(id),
    amount bigint NOT NULL CHECK (amount > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz
);

CREATE INDEX IF NOT EXISTS debt_repayments_debt_created_idx
    ON debt_repayments (debt_id, created_at DESC, id DESC)
    WHERE deleted_at IS NULL;
```

The `debts_amounts_check` constraint is the last line of defence: even a
buggy concurrent path cannot push the remaining amount below zero.

### Limits

| Field | Rule |
|---|---|
| `person_name` | Trimmed, 1–100 characters |
| `person_phone` | Optional; when present, valid per `helpers.NormalizePhone` and stored normalized |
| `amount` (USD) | 1 – 1,000,000,000 |
| `amount` (UZS) | 1 – 1,000,000,000,000 |

## API

All routes are under `/api/v1/debts`, behind `SessionJWTMiddleware`. Every
operation is scoped to the authenticated user. A debt that does not exist,
is deleted, or belongs to someone else returns `404`, so its existence is not
disclosed.

| Method | Path | Screen |
|---|---|---|
| `GET` | `/debts/summary` | Total summary cards |
| `GET` | `/debts` | List, filters, search |
| `POST` | `/debts` | Add Debt |
| `GET` | `/debts/:debtID` | Debt Details |
| `POST` | `/debts/:debtID/repayments` | Confirm Repayment |
| `GET` | `/debts/:debtID/repayments` | History tab |
| `POST` | `/debts/:debtID/complete` | Mark as Completed |
| `DELETE` | `/debts/:debtID` | Delete |

### Debt object

Returned by list, detail, create, repayment and complete:

```json
{
  "id": "uuid",
  "direction": "they_owe_me",
  "person_name": "Akmal",
  "person_phone": "",
  "currency": "USD",
  "original_amount": 1500,
  "remaining_amount": 900,
  "status": "active",
  "source": "manual",
  "created_at": "2026-10-01T09:00:00Z",
  "completed_at": null
}
```

`source` is `manual` for every debt in this stage; stage 2 adds `order`.

### `GET /debts/summary`

Sums `remaining_amount` of the user's active, non-deleted debts:

```json
{
  "usd": {"they_owe_me": 2500, "i_owe": 800},
  "uzs": {"they_owe_me": 1975000, "i_owe": 720000}
}
```

### `GET /debts`

| Query | Values | Default |
|---|---|---|
| `direction` | `they_owe_me`, `i_owe` | both |
| `status` | `active`, `completed`, `all` | `active` |
| `query` | Up to 100 characters; matches `person_name` or `person_phone` case-insensitively, LIKE wildcards escaped | none |
| `limit` / `offset` | 1–100 / 0–10,000 | 50 / 0 |

Newest first by `(created_at DESC, id DESC)`.

### `POST /debts`

```json
{"direction": "they_owe_me", "person_name": "Akmal", "person_phone": "", "currency": "USD", "amount": 1500}
```

Creates an active debt with `remaining_amount = original_amount = amount`.
Returns `201` with the debt.

### `POST /debts/:debtID/repayments`

```json
{"amount": 600}
```

In one transaction: lock the debt (`FOR UPDATE`), require `status = active`,
require `0 < amount ≤ remaining_amount`, insert the repayment, decrease
`remaining_amount`, and complete the debt when it reaches 0. Returns `201` with
the updated debt.

### `GET /debts/:debtID/repayments`

Newest first, same paging as the list:

```json
[{"id": "uuid", "amount": 600, "created_at": "2026-10-01T10:00:00Z"}]
```

Repayments of a completed debt remain visible. A deleted debt returns `404`.

### `POST /debts/:debtID/complete`

Locks the debt; requires `status = active`; sets `status = completed` and
`completed_at`. Returns `200` with the debt.

### `DELETE /debts/:debtID`

Soft-deletes the debt. Its repayments stay in the database but are no longer
reachable. Returns `200` with a localized message.

## Errors

Every client error uses `apperror.New` with Uzbek, Russian and English text,
as described in `ERRORS.md`.

| Code | When |
|---|---|
| 400 | Unknown direction, currency or status; empty or too long name; invalid phone; amount out of range; repayment of 0, negative or above the remaining amount; bad paging |
| 404 | Debt not found, deleted, or owned by someone else |
| 409 | Repaying or completing a debt that is already completed |

## Code layout

- `internal/debt/debt.go` — types, validation, limits.
- `internal/debt/service.go` — create, list, get, summary, complete, delete.
- `internal/debt/repayment.go` — record and list repayments.
- `internal/debt/handler.go`, `internal/debt/swagger.go` — HTTP layer.
- `pkg/routes/private_routes.go` — routes.
- `pkg/responses/envelope.go` — new localized messages.
- `DEBT_API.md` (Uzbek) and Swagger.

## Testing

Service tests against the test database:

- Ownership: another user gets `404` for get, repay, history, complete and
  delete, and never sees the debt in the list or summary.
- Create validation: each limit above.
- Partial repayments reduce the remaining amount; the final one completes the
  debt; overpaying is rejected.
- Two concurrent repayments that together exceed the remaining amount: one
  succeeds, the other fails, and the remaining amount never goes negative.
- Mark as Completed keeps the remaining amount; repaying or completing again
  returns `409`.
- Deleted debts disappear from list, summary and detail.
- Summary sums active debts per currency and direction and ignores completed
  and deleted ones.
- List filters (direction, status, query including `%` and `_`) and paging.
