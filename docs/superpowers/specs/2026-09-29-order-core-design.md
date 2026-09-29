# Order Core — Design

Stage 1 of 3 for the order feature. Stage 2 is two-party cancellation with
reversal; stage 3 is attachments. Both get their own specs.

This stage also carries one unrelated small fix: graceful shutdown hardening.

## Goal

Two employees in a group move money for customers (hawala style): the **giver**
takes cash from a customer in one city, the **receiver** pays a customer in
another. Each employee independently confirms the amount that actually changed
hands. When both confirmations agree, the order completes and balances and
profits update atomically. When they disagree, the order shows an amount
mismatch until someone corrects their confirmation.

Screens covered: Create Order, Edit Order, Order Details (confirm received
amount), Amount Mismatch, order history.

## Decisions

| Topic | Decision |
|---|---|
| Who creates/edits | Manager or owner for any two employees; an employee only for orders where they are giver or receiver. Investors read only. |
| Parties | Giver and receiver are distinct, active **employee** members of the group. |
| Balance effect on completion | Giver `balance_usd += amount`, receiver `balance_usd -= amount`. Balances may go negative. |
| Fee / profit | Optional. Each party enters the fee *they* collected (0 allowed) in their own confirmation. On completion each party's fee is added to their own `member_profit_periods` row. The fee entered at creation pre-fills the **giver's** confirmation. |
| Mismatch | Compared on `amount_usd` only, never on fees. |
| Customer phones | Both required. Found or created in `customers` by `(group_id, phone)`. |
| Money units | Whole USD and whole UZS in `bigint`, matching existing balance/profit columns. |
| Profit month | Month of `completed_at` in `Asia/Tashkent`. |
| Legacy orders | The 8 existing seed rows are deleted by the migration. |
| Architecture | New `internal/order` package in the style of `internal/group`: `Service` over `pgxpool` with hand-written SQL, plus `Handler`. The sqlc-based order stub is removed. |
| Ledger tables | Not added. Stage 2 reversal is computed from `order_confirmations`; history lives in `order_events`. |

## Data model

### Migration `14_orders.up.sql`

Legacy cleanup is guarded because `make migrate` and the test helper re-apply
every migration on each run:

```sql
DO $schema$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns
               WHERE table_schema = current_schema()
                 AND table_name = 'orders' AND column_name = 'total') THEN
        DELETE FROM orders;
        ALTER TABLE orders DROP COLUMN total;
    END IF;
END
$schema$;
```

Then idempotent `ALTER TABLE orders ADD COLUMN IF NOT EXISTS …` statements and
`CREATE TABLE IF NOT EXISTS` for the new tables, so the migration is safe on a
fresh database (where `03_orders` first creates the legacy shape) and on
re-runs.

### `orders`

| Column | Type | Notes |
|---|---|---|
| `id` | uuid PK | existing |
| `group_id` | uuid NOT NULL → `groups` | existing column, becomes NOT NULL |
| `created_by` | uuid NOT NULL → `users` | |
| `giver_member_id` | uuid NOT NULL | FK `(group_id, giver_member_id)` → `group_members(group_id, id)` |
| `receiver_member_id` | uuid NOT NULL | FK `(group_id, receiver_member_id)` → `group_members(group_id, id)` |
| `giver_location_id` | uuid NULL → `locations` | Snapshot of the giver's assigned location at creation/edit |
| `receiver_location_id` | uuid NULL → `locations` | Same for receiver |
| `giver_customer_id` | uuid NOT NULL → `customers` | |
| `receiver_customer_id` | uuid NOT NULL → `customers` | |
| `amount_usd` | bigint NOT NULL | Expected amount, `CHECK (amount_usd > 0)` |
| `fee_uzs` | bigint NOT NULL DEFAULT 0 | Fee entered at creation, `CHECK (fee_uzs >= 0)` |
| `status` | varchar(16) NOT NULL DEFAULT `'pending'` | `CHECK (status IN ('pending','completed','cancelled'))` |
| `completed_at`, `cancelled_at` | timestamptz NULL | |
| `created_at`, `updated_at`, `deleted_at` | timestamptz | existing audit columns |

Constraints and indexes:
- `CHECK (giver_member_id <> receiver_member_id)`
- `orders_group_created_idx` already covers `(group_id, created_at DESC)`; add
  `orders_giver_idx (giver_member_id)` and `orders_receiver_idx (receiver_member_id)`,
  both `WHERE deleted_at IS NULL`, for the employee-scoped list.

`status` uses `varchar + CHECK` (the `group_invitations` pattern) so stage 2 can
add values without enum migrations.

### `order_confirmations`

| Column | Type | Notes |
|---|---|---|
| `id` | uuid PK | |
| `order_id` | uuid NOT NULL → `orders` | |
| `member_id` | uuid NOT NULL → `group_members` | Giver or receiver |
| `amount_usd` | bigint NOT NULL | `CHECK (amount_usd > 0)` |
| `fee_uzs` | bigint NOT NULL DEFAULT 0 | `CHECK (fee_uzs >= 0)` |
| `confirmed_at` | timestamptz NOT NULL | Updated on correction |
| `created_at`, `updated_at`, `deleted_at` | timestamptz | Required by the schema test |

Unique index `(order_id, member_id) WHERE deleted_at IS NULL`. A correction
updates the active row. An order edit soft-deletes the active rows.

### `order_events`

| Column | Type |
|---|---|
| `id` | uuid PK |
| `order_id` | uuid NOT NULL → `orders` |
| `actor_user_id` | uuid NOT NULL → `users` |
| `event_type` | varchar(50) NOT NULL |
| `payload` | jsonb NOT NULL DEFAULT `'{}'` |
| `created_at`, `updated_at`, `deleted_at` | timestamptz |

Index `(order_id, created_at, id)`.

Event types: `created`, `updated`, `confirmed`, `confirmation_corrected`,
`amount_mismatch`, `completed`.

Payloads never contain confirmation amounts or fees, because events are visible
to both parties and would leak the counterparty's numbers. `updated` carries
`{"changes": {"<field>": {"old": …, "new": …}}}` for order fields;
`completed` carries `{"amount_usd": …}`.

## API

All routes sit behind `SessionJWTMiddleware` under
`/api/v1/groups/:groupID/orders`. The global `/api/v1/orders` routes are
removed.

### Visibility

- Owner, manager, investor: every order in the group.
- Employee: only orders where they are giver or receiver. Other orders return
  `404`, not `403`, so their existence is not disclosed.
- Non-members: `404`, matching the existing `requireMember` behaviour.

### `POST /orders` — create

```json
{
  "giver_user_id": "uuid",
  "giver_customer_phone": "+998901234567",
  "receiver_user_id": "uuid",
  "receiver_customer_phone": "+998901234567",
  "amount_usd": 7000,
  "fee_uzs": 50000
}
```

`fee_uzs` is optional (default 0). The creator is not auto-confirmed. Response
`201` with the order detail body. Writes a `created` event and notifies the
party that is not the actor (both parties when a manager creates).

### `GET /orders?status=&limit=&offset=` — list

`status` is optional (`pending|completed|cancelled`). `limit` defaults to 50,
maximum 100; `offset` 0–10000 — same parsing rules as notifications. Newest
first by `(created_at DESC, id DESC)`.

List item:

```json
{
  "id": "uuid",
  "status": "pending",
  "state": "waiting_for_you",
  "amount_usd": 7000,
  "giver":    {"user_id": "uuid", "name": "Javohir", "location_name": "Tashkent"},
  "receiver": {"user_id": "uuid", "name": "Aziz",    "location_name": "Kokand"},
  "created_at": "2026-09-29T10:42:00Z"
}
```

### `GET /orders/:orderID` — detail

```json
{
  "id": "uuid",
  "group_id": "uuid",
  "status": "pending",
  "state": "amount_mismatch",
  "amount_usd": 7000,
  "fee_uzs": 50000,
  "giver": {
    "user_id": "uuid", "name": "Javohir", "avatar_url": "",
    "location": {"id": "uuid", "name": "Tashkent"},
    "customer_phone": "+998901234567"
  },
  "receiver": { "…": "same shape" },
  "created_by": {"user_id": "uuid", "name": "Javohir"},
  "confirmations": [
    {"role": "giver",    "user_id": "uuid", "status": "confirmed", "amount_usd": 7000, "fee_uzs": 50000, "confirmed_at": "…"},
    {"role": "receiver", "user_id": "uuid", "status": "confirmed", "amount_usd": 6950}
  ],
  "created_at": "…", "updated_at": "…", "completed_at": null
}
```

`location` is `null` when no location was assigned.

`state` is computed per viewer:

| Condition | `state` |
|---|---|
| `status = completed` | `completed` |
| `status = cancelled` | `cancelled` |
| pending, both confirmed, amounts differ | `amount_mismatch` |
| pending, viewer is a party and has not confirmed | `waiting_for_you` |
| otherwise pending | `waiting_for_confirmation` |

Confirmation field visibility:
- A party always sees their own `amount_usd` and `fee_uzs`.
- A party sees the counterparty's `amount_usd` only after confirming themselves,
  so nobody can copy the other side's number.
- A party never sees the counterparty's `fee_uzs` (profit is personal, matching
  `my_profit_uzs`).
- Owner, manager and investor see everything.

Hidden fields are omitted from the JSON.

### `PATCH /orders/:orderID` — edit

Same fields as create, all optional, at least one required. Allowed only while
`status = pending`; otherwise `409`. Same permission rule as create, evaluated
against the order **after** the edit, so an employee cannot edit themselves out
of an order.

If any field actually changes: snapshot locations again when a party changes,
soft-delete both active confirmations, write an `updated` event with the diff,
and notify the parties other than the actor. A no-op edit returns `200` and
writes nothing.

### `POST /orders/:orderID/confirmations` — confirm or correct

```json
{"amount_usd": 7000, "fee_uzs": 50000}
```

Only the giver or receiver, for themselves; anyone else gets `403` (if they can
see the order) or `404`. Allowed only while `pending`; otherwise `409`.

In one transaction:

1. Lock the order: `SELECT … FROM orders WHERE id = $1 AND group_id = $2 AND deleted_at IS NULL FOR UPDATE`.
2. Insert the actor's confirmation, or update the active one (event
   `confirmed` or `confirmation_corrected`).
3. If both parties now have active confirmations:
   - **Equal `amount_usd`** → complete:
     - `UPDATE orders SET status = 'completed', completed_at = now`.
     - Giver balance `+amount`, receiver balance `-amount`, via
       `INSERT INTO employee_balances … ON CONFLICT (group_id, member_id) WHERE deleted_at IS NULL DO UPDATE SET balance_usd = employee_balances.balance_usd + EXCLUDED.balance_usd`.
     - For each party with `fee_uzs > 0`, the same upsert into
       `member_profit_periods` keyed by `(group_id, member_id, year, month)`
       of `completed_at AT TIME ZONE 'Asia/Tashkent'`.
     - `completed` event; notify both parties.
   - **Different amounts** → `amount_mismatch` event; notify both parties.
4. Commit. Respond `200` with the order detail.

The row lock serialises confirmations and edits on the same order, so effects
apply exactly once even when both parties confirm at the same moment.

### `GET /orders/:orderID/events` — history

Oldest first, no pagination (orders produce a handful of events).

```json
[{"id": "uuid", "event_type": "created", "actor": {"user_id": "uuid", "name": "Javohir"}, "payload": {}, "created_at": "…"}]
```

## Notifications

Written with the existing `notifications` + `notification_recipients` tables,
inside the same transaction as the change, with `uz`/`ru`/`en` titles and
content. Payload: `{"event_type": "...", "order_id": "...", "group_id": "..."}`.

| Event type | Recipients |
|---|---|
| `ORDER_CREATED` | Parties other than the actor |
| `ORDER_UPDATED` | Parties other than the actor |
| `ORDER_AMOUNT_MISMATCH` | Both parties |
| `ORDER_COMPLETED` | Both parties |

## Validation and errors

| Rule | Error |
|---|---|
| `amount_usd` 1 – 1,000,000,000 | 400 |
| `fee_uzs` 0 – 1,000,000,000,000 | 400 |
| Phones valid per `helpers.NormalizePhone` | 400 |
| Giver ≠ receiver | 400 |
| Party is not an active employee member of the group | 404 |
| Employee creates/edits an order they are not a party to | 403 |
| Investor creates/edits | 403 |
| Manager or investor confirms | 403 |
| Edit or confirm a non-pending order | 409 |

Customer creation uses `INSERT … ON CONFLICT (group_id, phone) WHERE deleted_at IS NULL DO UPDATE SET updated_at = EXCLUDED.updated_at RETURNING id`,
so concurrent orders with the same phone do not collide.

Out of scope for stage 1:
- A party leaving the group while an order is pending leaves it pending; stage 2
  cancellation resolves it.
- Duplicate submission (double tap) creates two orders; the client disables the
  button while the request is in flight. No idempotency keys.

## Code layout

- `internal/order/service.go` — `Service`, create/list/get/edit/confirm.
- `internal/order/effects.go` — completion: balance and profit upserts.
- `internal/order/notifications.go` — translations and notification inserts.
- `internal/order/handler.go`, `internal/order/swagger.go` — HTTP layer and
  Swagger response types, mirroring `internal/group`.
- Remove `internal/order/{dto,repository,usecase,handler/rest}` and
  `db/sqlc/query/orders.sql`; regenerate sqlc.
- `pkg/routes/private_routes.go` — replace `/orders` routes with the group-nested
  routes.
- Regenerate Swagger; add `ORDER_API.md` (Uzbek, same style as `GROUP_API.md`)
  for the mobile team.

## Graceful shutdown fix

In `internal/app/server.go` and `utils/start_server.go`:
- Replace `restApp.Shutdown()` with `restApp.ShutdownWithTimeout(15 * time.Second)`
  so a stuck request cannot block shutdown past the orchestrator's 30 s grace
  period (15 s drain plus the existing 10 s hook budget).
- Call `signal.Stop` after the first signal so a second Ctrl+C terminates
  immediately.

## Testing

Service tests against the test database, following `internal/group/service_test.go`:

- Create permission matrix: manager for others ✓, employee as party ✓,
  employee for others 403, investor 403, non-member 404, non-employee party 404.
- Customers are created on first use and reused on the second order.
- Matching confirmations → `completed`; giver and receiver balances and each
  party's profit row in the correct Tashkent month.
- Mismatch → correction → `completed`.
- Edit clears confirmations and records the diff; no-op edit writes nothing;
  editing a completed order → 409; employee cannot edit themselves out.
- Concurrent confirmations from both parties (two goroutines) apply effects
  exactly once.
- Visibility: employee gets 404 for others' orders; counterparty amount hidden
  before own confirmation; counterparty fee never shown to a party.
- Notifications written for each event with the right recipients.
- Migration: legacy `total` rows are cleared; applying all migrations twice is a
  no-op.

Handler tests cover path/query parsing and status codes for bad input.
