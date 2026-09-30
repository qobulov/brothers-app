# Member Management — Design

Stage 1 of 2 for the group member screens. Stage 2 is profit settlement and
its history; it changes how "current profit" is calculated and gets its own
spec.

## Goal

Let a group see and manage one member: their profile, balance and profit,
their role and location, manual balance corrections with a permanent history,
and removal from the group when nothing is left unsettled.

Screens covered: Member Details, Adjust Balance, Balance History, Edit Member,
Remove from Group.

## Decisions

| Topic | Decision |
|---|---|
| Who sees a member | Owner, manager and investor see any member. An employee sees only themselves. |
| Adjust balance | Any manager. Target must be an active employee. |
| Change location | Any manager. Only employees have a location. |
| Change role | Owner only. The owner's own role cannot change. |
| Remove member | Any manager removes an employee. Only the owner removes a manager or investor. Nobody removes the owner or themselves. |
| Removal and leaving the employee role | Require a $0 balance and no active orders. |
| Active order | The member is giver or receiver of a pending order, or of any order with an open cancellation request. Having only created an order does not count. |
| Location already taken | 409. The manager moves the other employee first. |
| Balance history | Manual adjustments only, as the design shows. Order effects stay in order history. |
| Contact field | Users have an email, not a phone. Phones belong to customers only. |
| Profit shown | All-time total for now. Stage 2 changes it to profit since the last settlement. |
| Architecture | New files in `internal/group`, same `Handler` + `Service` pattern with hand-written SQL. |

## Data model

### Migration `16_balance_adjustments.up.sql`

```sql
CREATE TABLE IF NOT EXISTS balance_adjustments (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id uuid NOT NULL,
    member_id uuid NOT NULL,
    old_balance_usd bigint NOT NULL,
    new_balance_usd bigint NOT NULL,
    amount_usd bigint NOT NULL,
    reason varchar(500),
    created_by uuid NOT NULL REFERENCES users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz,
    CONSTRAINT balance_adjustments_group_member_fkey
        FOREIGN KEY (group_id, member_id) REFERENCES group_members(group_id, id),
    CONSTRAINT balance_adjustments_amount_check
        CHECK (amount_usd = new_balance_usd - old_balance_usd AND amount_usd <> 0)
);

CREATE INDEX IF NOT EXISTS balance_adjustments_member_created_idx
    ON balance_adjustments (group_id, member_id, created_at DESC, id DESC)
    WHERE deleted_at IS NULL;
```

Rows are never updated or deleted. A wrong adjustment is corrected with
another adjustment.

No other schema changes. Role changes and removals are recorded in the
existing `audit_logs` table.

## API

All routes are under `/api/v1/groups/:groupID/members/:userID`, behind
`SessionJWTMiddleware`. The member is identified by user ID.

| Method | Path | Allowed |
|---|---|---|
| `GET` | `/members/:userID` | Owner, manager, investor: any member. Employee: only themselves. |
| `PATCH` | `/members/:userID` | Role: owner. Location: any manager. |
| `DELETE` | `/members/:userID` | Employee target: any manager. Manager or investor target: owner. |
| `POST` | `/members/:userID/balance-adjustments` | Any manager |
| `GET` | `/members/:userID/balance-adjustments` | Same as member detail |

"Manager" means `is_owner` or role `manager` (plus the legacy `owner` and
`admin` roles), matching `requireManager`.

Non-members get `404`. An employee asking for another member gets `404`.

### `GET /members/:userID` — member detail

```json
{
  "member_id": "uuid",
  "user_id": "uuid",
  "full_name": "Aziz Karimov",
  "username": "aziz",
  "email": "aziz@example.com",
  "avatar_url": "",
  "role": "employee",
  "is_owner": false,
  "location": {"id": "uuid", "name": "Kokand"},
  "balance_usd": 7200,
  "profit_uzs": 940000,
  "joined_at": "2026-09-01T09:00:00Z",
  "removal": {"allowed": false, "balance_is_zero": false, "no_active_orders": true},
  "permissions": {
    "can_edit_role": true,
    "can_edit_location": true,
    "can_adjust_balance": true,
    "can_remove": true
  }
}
```

- `location` is `null` when none is assigned.
- `balance_usd` and `profit_uzs` are present for employees only.
- `removal.allowed` is `balance_is_zero AND no_active_orders`. It describes the
  member's state, not the viewer's rights.
- `permissions` describes what the **viewer** may attempt on this member, from
  the rules above (role of viewer and target, owner flag, not self). It does
  not include the state checks in `removal`.

### `PATCH /members/:userID` — edit

```json
{"role": "manager", "location_id": "uuid"}
```

Both fields are optional; at least one is required. `location_id: ""`
unassigns the location. Returns the updated member detail.

Role rules:
- `role` is `employee`, `manager` or `investor`.
- Owner only, and the target must not be the owner.
- Leaving `employee` requires a $0 balance and no active orders, and unassigns
  the member's location. The balance row stays at 0.
- Becoming `employee` creates a $0 balance row if none exists.

Location rules:
- Any manager. After the role change in the same request, the target must be
  an employee; otherwise `400`.
- The location must belong to the group and not be deleted; otherwise `404`.
- A location held by another employee → `409`.
- Balance and profit are not touched.

A request that changes nothing returns `200` and writes nothing. A role change
writes an `audit_logs` row (`action = 'member.role_changed'`, old and new role).

### `DELETE /members/:userID` — remove

Runs in one transaction with the member row locked:

1. Permission by target role; target is not the owner and not the actor.
2. Balance is 0 and there are no active orders; otherwise `409` with a reason
   naming the blocker.
3. Soft-delete the membership, clear `locations.employee_id` for their
   location, write `audit_logs` (`action = 'member.removed'`).

The user can be invited again; acceptance creates a new membership. Their old
orders keep pointing at the removed membership.

Follow-up in the order package: `RequestCancellation` returns `409` when either
party is no longer an active member, so a removed member's old orders cannot be
left with a cancellation request nobody can answer.

### `POST /members/:userID/balance-adjustments` — adjust balance

```json
{"new_balance_usd": 7500, "reason": "Cash correction"}
```

In one transaction:

1. Target is an active employee; otherwise `404`.
2. Ensure the balance row exists, then `SELECT … FOR UPDATE` it. Order
   completion updates the same row, so neither change can be lost.
3. `new_balance_usd` must differ from the current balance and be within
   ±1,000,000,000,000; `reason` is optional, trimmed, at most 500 characters.
4. Update the balance and insert the adjustment.

Returns `201` with the adjustment (same shape as a history item).

### `GET /members/:userID/balance-adjustments` — balance history

`limit` defaults to 50, maximum 100; `offset` 0–10000. Newest first.

```json
{
  "member": {
    "user_id": "uuid", "full_name": "Aziz Karimov", "avatar_url": "",
    "role": "employee", "location_name": "Kokand"
  },
  "current_balance_usd": 7500,
  "adjustments": [
    {
      "id": "uuid",
      "direction": "increase",
      "amount_usd": 300,
      "old_balance_usd": 7200,
      "new_balance_usd": 7500,
      "reason": "Cash correction",
      "changed_by": {"user_id": "uuid", "name": "Abror"},
      "created_at": "2026-09-30T14:32:00Z"
    }
  ]
}
```

`direction` is `increase` or `decrease`. `reason` is omitted when empty. For a
member who is not an employee, `current_balance_usd` is 0 and `adjustments`
holds whatever history exists from when they were one.

## Errors

| Code | When |
|---|---|
| 400 | Unknown role, bad ID, empty edit, location for a non-employee, new balance equals the current one or out of range, reason too long |
| 403 | The viewer can see the member but may not do this |
| 404 | Not in the group, member or location not found, employee opening another member |
| 409 | Non-zero balance or active orders block removal or the role change; location already taken; target is the owner; removing yourself |

## Code layout

- `internal/group/member_detail.go` — detail query, removal state, permissions.
- `internal/group/member_manage.go` — edit (role, location) and remove.
- `internal/group/balance.go` — adjust balance and balance history.
- `internal/group/member_handler.go` — the five handlers; Swagger types in
  `swagger.go`.
- `internal/order/cancel.go` — reject cancellation when a party has left.
- `pkg/routes/private_routes.go` — new routes.
- `GROUP_API.md` and Swagger updated.

## Testing

Service tests against the test database, following the existing group tests:

- Permission matrix for each action: owner, manager, investor, employee, outsider.
- Detail: employees see only themselves; money fields only for employees;
  `removal` and `permissions` reflect state and viewer.
- Adjust balance: records old, new and difference; equal balance rejected;
  non-employee target rejected; an order completing concurrently is not lost.
- History: newest first, paging, `direction`, reason omitted when empty.
- Role: leaving employee blocked by balance and by an active order; success
  unassigns the location; becoming employee creates a $0 balance; owner's role
  is fixed; manager cannot change roles.
- Location: assign, unassign, taken by another (409), non-employee (400),
  foreign or deleted location (404).
- Removal: each blocker on its own, then success; location freed; re-invite
  works; the removed member's old completed order can no longer be cancelled.
