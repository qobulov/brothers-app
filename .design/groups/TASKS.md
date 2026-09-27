# Group management implementation plan

This is an API/database-first plan for the supplied mobile Group screens. Each
slice is releasable and keeps access control at the API boundary.

## 1. Lock the domain contract and permission matrix

- Create a short ADR in `.design/groups/` that fixes the three membership
  roles (`manager`, `employee`, `investor`), the owner-as-manager flag, their
  capabilities, and the exact order direction/ledger semantics.
- Reuse the authenticated `users` identity. Treat email and username as the
  invitation lookup keys; do not introduce a phone requirement.
- Define the explicit legacy-role backfill: `owner`/`is_owner` → owner,
  `admin` → manager, and confirm whether legacy `member` → employee is safe.
- Confirm investor PII visibility and whether an employee can be assigned to
  more than one location before the migration is written.

**Done when:** one permission matrix governs handlers, services, and tests; no
role decision is left implicit in a handler.

## 2. Add the group domain migration and SQLC queries

- Create a new forward-only migration; do not rewrite the existing auth/group
  foundation migration.
- Replace the legacy membership role model with a migration-safe
  `group_member_role` field, retain old values for history, and enforce one
  active manager-owner per group with a partial unique index.
- Create `group_locations`, member-location assignment, invitation recipient
  fields (user/email), invitation status/audit fields, and `group_audit_logs`.
- Extend `orders` with group, location, creator/participant **membership**
  IDs, status, timestamps, customer phone snapshot, and minor-unit monetary
  columns. Add immutable order/ledger entries once the financial decision from
  task 1 is approved.
- Add partial unique/indexes for active memberships, location names within a
  group, group/status order listing, invitation lookup, and soft-deleted rows.
- Update `db/sqlc/schema.sql`, group/location/order query files, regenerate
  models, and use UUID/decimal-safe Go types consistently.

**Done when:** a clean database can migrate, existing group rows backfill
deterministically, and SQLC exposes only parameterized group-scoped queries.

## 3. Deliver the basic group workspace APIs

- Create the `internal/group` module (entity, repository, service, handler,
  request/response DTOs) using the project’s existing auth/response patterns.
- Implement `POST /api/v1/groups`: create the group and creator owner
  membership in one transaction.
- Implement paginated/searchable `GET /api/v1/groups` for the Groups cards,
  returning the caller’s role, member/location counts, and their financial
  summary.
- Implement `GET /api/v1/groups/{group_id}` for the group header, quick access
  permissions, and current member summary. Add `PATCH` for permitted group
  metadata changes.
- Add middleware/service helpers that resolve an active membership from
  `group_id`; every later group endpoint must use this helper.

**Done when:** an authenticated user can only list/open groups they belong to,
and group creation atomically creates exactly one owner.

## 4. Deliver invitation and member-management flow

- Implement member listing/search/filtering:
  `GET /groups/{group_id}/members?role=&query=&cursor=`.
- Implement registered-user lookup by username/email without leaking users who
  are not eligible to be invited.
- Implement `POST /groups/{group_id}/invitations`, list/revoke endpoints, and
  authenticated accept/reject endpoints. The accepted invitation creates the
  membership and employee-location assignment atomically.
- Implement `PATCH /groups/{group_id}/members/{member_id}` for allowed role or
  location changes, with owner/manager policy checks and an audit event.
- Reuse the existing notification infrastructure where available; otherwise
  expose an in-app pending-invitations endpoint rather than adding new config.

**Done when:** the Invite Member screen can find a registered user, assign an
allowed role/location, and the user can accept exactly once.

## 5. Deliver location management and safety checks

- Create group-scoped `GET`, `POST`, `PATCH`, and soft-delete location
  endpoints under `/groups/{group_id}/locations`.
- Permit owner/manager edits; allow employee/investor read access only where
  required by their group view.
- Assign/unassign employees through a transaction that validates the employee
  role and the single-location rule chosen in task 1.
- Before deleting a location, return a precise conflict reason such as
  `location_has_assigned_employee` or `location_has_active_orders`, matching the
  warning shown in the design.

**Done when:** the Locations screen can show assignees, add a location, and
receive actionable blockers instead of a generic internal error.

## 6. Replace global orders with group-scoped order lifecycle

- Create a group order service and migrate the simple public `/orders` CRUD to
  authenticated routes under `/groups/{group_id}/orders`; retire or lock down
  the old public routes after client migration.
- Implement create, detail, paginated list/search by phone or amount, and
  status filters for pending/completed/cancelled.
- Enforce role/location visibility: owner/manager see authorized group work,
  employees are limited to their assigned-location/own workflow, and investors
  remain read-only.
- Make complete/cancel transitions transactional, validate valid state changes,
  and write the corresponding financial/audit entries exactly once.
- Return the card fields required by the screen: amounts, parties, phone (only
  when allowed), status, and time.

**Done when:** one group cannot read or mutate another group’s orders, and
concurrent state changes cannot double-apply financial effects.

## 7. Implement summaries, reports, and balances

- Create group/member summary queries for the group cards and detail balance
  tiles: member count, location count, the caller’s USD balance, and UZS
  profit. Add member-level profit periods because the design exposes “My
  Profit”, not only group total profit.
- Create report endpoints appropriate for owner/manager/investor permissions;
  omit customer PII for investors unless task 1 explicitly permits it.
- Derive amounts from the ledger/order history or update a guarded summary in
  the same transaction. Never calculate financial values with `float64`.
- Add indexes and cursor pagination suitable for the list and report views.

**Done when:** the values on Groups, group detail, and member cards agree with
the same underlying financial history.

## 8. Implement ownership, removal, and deletion workflows

- Implement `POST /groups/{group_id}/ownership-transfer` with row locking:
  target must be an active manager, target becomes owner, former owner becomes
  manager, and an audit record is created.
- Implement a removal preflight and `DELETE /groups/{group_id}/members/{id}`.
  It must return structured blockers for non-zero balance, active orders,
  assigned location, and attempted owner removal.
- Implement owner-only `DELETE /groups/{group_id}` as a soft delete. Require
  pending orders to be resolved, revoke active invitations/access, and retain
  orders/ledger/audit history.
- Ensure all lifecycle operations are idempotent or return a meaningful
  conflict code, never an ambiguous 1500 response.

**Done when:** the three overflow actions in the designs have server-enforced
rules even if a client bypasses its own warning dialogs.

## 9. Standardize API documentation, errors, and localization

- Add Swagger/OpenAPI documentation for every group, invitation, location, and
  group-order route, including request examples and role requirements.
- Reuse the application response/error format and localized message handling.
  Define actionable slugs: `group_not_found`, `not_group_member`,
  `insufficient_group_role`, `invitation_already_pending`,
  `member_has_balance`, `member_has_active_orders`,
  `location_has_assigned_employee`, and `invalid_order_transition`.
- Keep internal database/provider details out of public responses while the
  existing error-reporting path receives the wrapped cause, request ID, actor,
  group ID, and route.

**Done when:** clients can render every blocked-state message from a stable
status/slug and operators can trace the original failure by request ID.

## 10. Verify, roll out, and remove legacy exposure

- Add repository/service/handler integration tests for every role, cross-group
  isolation, invitation replay, transfer races, deletion/removal blockers, and
  complete/cancel idempotency.
- Test migration/backfill using a snapshot containing legacy groups, members,
  invitations, and orders.
- Deploy schema first, then group APIs behind the authenticated contract, then
  migrate the mobile client, and finally disable the legacy public order CRUD.
- Run `go test ./...`, SQLC generation, migration checks, and a manual API
  smoke test for the four designed flows: create group, invite employee, create
  order, transfer/remove/delete.

**Done when:** existing data survives the rollout, no public endpoint exposes
group orders, and the primary mobile flows pass against a fresh and upgraded
database.
