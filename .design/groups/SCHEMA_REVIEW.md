# Group schema review

The supplied schema is a strong domain starting point. Apply the following
changes before writing the PostgreSQL migration.

## Required corrections

| Area | Problem | Required schema rule |
| --- | --- | --- |
| User identity | `users.phone` and `trial_claims.phone` are required, but the application now authenticates by email and phone is optional. | Keep `users.email` as required and unique; make `users.phone` nullable. Use `invited_user_id` and/or email for invitations. Do not make phone a login prerequisite. |
| Owner | `user_role` has no `owner`, while `is_owner` is separate. | Keep the UI meaning: an owner is a manager with `is_owner = true`; enforce one active owner per group and require owner rows to have role `manager`. Alternatively add `owner` to the enum, but never allow both representations to diverge. |
| Soft deletion | `group_members.deleted_at` is `not null`; `(group_id, user_id)` is globally unique. | Make `deleted_at` nullable and create `UNIQUE (group_id, user_id) WHERE deleted_at IS NULL` so a departed user may join again. Apply equivalent active-row indexes to locations and invitations. |
| Group isolation | Locations and orders reference `users.id`, so database FKs cannot prove that a user belongs to the same group or has the employee role. | Reference `group_members.id` for assigned employees and order parties (`employee_membership_id`, `from_member_id`, `to_member_id`), or use composite group/member FKs. Validate role and group in the service transaction. |
| Location assignment | `locations.employee_id` does not prevent one employee being assigned to conflicting locations. | Store `employee_member_id` and add an active partial unique index for the selected one-location-per-group rule. Unassign before member/location deletion. |
| Money | `amount_usd bigint` does not state its unit; USD values may accidentally be stored as dollars in one endpoint and cents in another. | Rename to `amount_usd_cents` and `profit_uzs_tiyin` (or explicitly choose whole UZS). All balance transaction amounts must be signed minor-unit integers. |
| Profit attribution | `group_profit_periods` stores only the group total, while the screens show **My Profit** for a member. | Add `member_profit_periods(group_id, member_id, year, month, profit_uzs_*)`, or make clear that every member sees the same group profit. The design implies the former. |
| Invitations | A phone-only invite plus token cannot model an in-app invitation reliably for a registered email user. | Add `invited_user_id`, optional `invited_email`, `status`, `responded_at`, `revoked_at`, and a partial unique index for one pending invitation per group/user. Token is optional only for an external accept link. |
| Customer snapshot | Both `customer_id` and `customer_phone` can become inconsistent. | Use `customer_id` as canonical; if an order must preserve a historical phone, name it `customer_phone_snapshot` and fill it atomically. |
| Financial writes | Balance rows, adjustments, and ledger transactions can be written inconsistently. | Treat `balance_transactions` and `profit_transactions` as immutable ledgers; `employee_balances` is a locked/materialized current-balance cache updated in the same transaction. Add source checks and unique order-event keys to prevent double completion/reversal. |

## Recommended enums

```text
group_member_role = employee | manager | investor
order_status       = pending | completed | cancelled
debt_currency      = USD | UZS
debt_direction     = I_OWE | THEY_OWE_ME
debt_status        = active | paid | cancelled
```

`user_role` should be renamed to `group_member_role`, because it does not
describe a user’s global identity or permissions.

## Essential database constraints

- Exactly one active group owner:
  `UNIQUE (group_id) WHERE is_owner AND deleted_at IS NULL`.
- One active membership for a user in a group:
  `UNIQUE (group_id, user_id) WHERE deleted_at IS NULL`.
- One active customer phone per group:
  `UNIQUE (group_id, phone) WHERE deleted_at IS NULL`.
- One active location name per group:
  `UNIQUE (group_id, lower(name)) WHERE deleted_at IS NULL`.
- One confirmation per order party, as the draft already proposes.
- Every order state transition, ledger insert, balance-cache update, profit
  period update, and audit log must run in one database transaction with rows
  locked by order ID.

## Completion rule for an order

1. Create the order as `pending`.
2. Each required participant creates one immutable confirmation.
3. When confirmation requirements are satisfied, lock the order and transition
   it once to `completed`.
4. In the same transaction write balance/profit ledger rows, update cached
   balances/periods, and insert an audit event.
5. Cancellation is allowed only from `pending`; a completed reversal requires
   an explicit, audited reversal flow rather than changing history in place.

## Recommended rollout order

1. Add new nullable columns/tables and indexes.
2. Backfill memberships, then enforce active-owner/member constraints.
3. Ship authenticated group endpoints and migrate clients from public orders.
4. Enable ledger-backed completion.
5. Remove/disable legacy public order access only after client cutover.
