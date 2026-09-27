# Group management and group orders

Date: 2026-09-27

## Goal

Implement the backend contract behind the supplied Groups screens: a user can
create and join groups, work within a location, create and view group orders,
and manage members according to their role.

The repository currently has database scaffolding for `groups`,
`group_members`, and `group_invitations`, but has no group API, location
model, membership policy, or group-scoped order model. This brief therefore
covers the API and database work; the supplied mobile screens are the client
reference.

## Product model

| Concept | Intended behaviour |
| --- | --- |
| Group | A soft-deletable workspace with one active owner and many members. |
| Owner | A manager with the active ownership flag; full control, including ownership transfer and group deletion. |
| Manager | Manages members and locations, but cannot transfer ownership or delete the group. |
| Employee | Works at one assigned location in a group and sees their own balance/profit. |
| Investor | Read-only access to permitted financial reports. |
| Location | A named unit inside a group. It may be assigned to one active employee. |
| Invitation | An in-app invitation to an already registered user, found by username or email. |
| Order | A group-scoped financial operation tied to a location and lifecycle state: pending, completed, or cancelled. |

## Required rules

- A group always has exactly one active owner. The owner must have the manager
  role, so ownership cannot drift from the role model.
- An ownership target must be an active manager in the same group. After a
  transfer, the former owner remains a manager.
- An employee needs an assigned location; an investor does not.
- A member cannot be removed while they have a non-zero USD balance, active
  orders, or an assigned location. Historical records remain available.
- A location cannot be removed while an employee is assigned to it or it has
  active orders.
- Group deletion is a soft delete and retains financial/audit history. Pending
  work must be resolved first.
- All group and order reads must be scoped to the requester's active membership;
  the existing public global order endpoints must not expose group data.

## Financial assumptions to confirm before migration

- USD balance and UZS profit use integer minor units (for example USD cents),
  never Go `float64`. Column names must state their units.
- Orders and immutable ledger entries are the source of financial history;
  summary balances are derived from them or maintained transactionally.
- The exact meaning of the order direction shown as “Javohir → Aziz”, and
  whether investors can see customer names and phone numbers, need product
  confirmation before the order schema is finalized.

## Compatibility constraints

- Authentication is email-based and `phone` is optional. Invitations must use
  a registered user ID and/or username/email rather than the legacy invitation
  phone field.
- Existing membership roles are `owner`, `admin`, and `member`; migration must
  preserve existing history and explicitly map legacy roles rather than mutate
  data implicitly.
- Existing groups/order data must stay readable during rollout.
