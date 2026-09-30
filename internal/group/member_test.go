package group

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qobulov/brothers-app/pkg/apperror"
	"github.com/qobulov/brothers-app/pkg/database"
)

// memberFixture is a group with an owner, a manager, an investor and two
// employees. Aziz works in Kokand; Javohir has no location.
type memberFixture struct {
	t       *testing.T
	pool    *pgxpool.Pool
	service *Service
	groupID uuid.UUID

	owner, manager, investor, aziz, javohir, outsider uuid.UUID
	azizMember, javohirMember                         uuid.UUID
	kokand, andijan                                   uuid.UUID
}

func newMemberFixture(t *testing.T) *memberFixture {
	t.Helper()
	pool, cleanup := database.SetupTestDB(t)
	t.Cleanup(cleanup)
	f := &memberFixture{t: t, pool: pool, service: NewService(pool)}
	f.owner = createUser(t, pool, "owner@example.com", "owner")
	f.manager = createUser(t, pool, "manager@example.com", "manager")
	f.investor = createUser(t, pool, "investor@example.com", "investor")
	f.aziz = createUser(t, pool, "aziz@example.com", "aziz")
	f.javohir = createUser(t, pool, "javohir@example.com", "javohir")
	f.outsider = createUser(t, pool, "outsider@example.com", "outsider")

	created, err := f.service.Create(context.Background(), f.owner, "Brothers")
	if err != nil {
		t.Fatalf("create group: %v", err)
	}
	f.groupID = created.ID
	addGroupMember(t, pool, f.groupID, f.manager, "manager")
	addGroupMember(t, pool, f.groupID, f.investor, "investor")
	f.azizMember = addGroupMember(t, pool, f.groupID, f.aziz, "employee")
	f.javohirMember = addGroupMember(t, pool, f.groupID, f.javohir, "employee")
	f.kokand = f.location("Kokand", &f.azizMember)
	f.andijan = f.location("Andijan", nil)
	return f
}

func (f *memberFixture) location(name string, memberID *uuid.UUID) uuid.UUID {
	f.t.Helper()
	var id uuid.UUID
	err := f.pool.QueryRow(context.Background(), `
		INSERT INTO locations (group_id, name, employee_id, created_by) VALUES ($1, $2, $3, $4) RETURNING id
	`, f.groupID, name, memberID, f.owner).Scan(&id)
	if err != nil {
		f.t.Fatalf("create location: %v", err)
	}
	return id
}

func (f *memberFixture) exec(sql string, args ...any) {
	f.t.Helper()
	if _, err := f.pool.Exec(context.Background(), sql, args...); err != nil {
		f.t.Fatalf("exec %q: %v", sql, err)
	}
}

// pendingOrder makes Aziz the giver and Javohir the receiver of a pending order.
func (f *memberFixture) pendingOrder() {
	f.t.Helper()
	f.exec(`INSERT INTO customers (group_id, phone) VALUES ($1, '+998901111111') ON CONFLICT DO NOTHING`, f.groupID)
	f.exec(`
		INSERT INTO orders (group_id, created_by, giver_member_id, receiver_member_id, giver_customer_id, receiver_customer_id, amount_usd)
		SELECT $1, $2, $3, $4, customers.id, customers.id, 7000
		FROM customers WHERE group_id = $1 LIMIT 1
	`, f.groupID, f.owner, f.azizMember, f.javohirMember)
}

func (f *memberFixture) detail(actor, target uuid.UUID) MemberDetail {
	f.t.Helper()
	detail, err := f.service.GetMember(context.Background(), actor, f.groupID, target)
	if err != nil {
		f.t.Fatalf("get member: %v", err)
	}
	return detail
}

func TestGetMember_DetailVisibilityAndPermissions(t *testing.T) {
	f := newMemberFixture(t)
	ctx := context.Background()
	addEmployeeBalance(t, f.pool, f.groupID, f.azizMember, 7200)
	f.exec(`INSERT INTO member_profit_periods (group_id, member_id, year, month, profit_uzs) VALUES ($1, $2, 2026, 9, 940000)`, f.groupID, f.azizMember)

	byOwner := f.detail(f.owner, f.aziz)
	if byOwner.FullName != "aziz" || byOwner.Email != "aziz@example.com" || byOwner.Role != "employee" || byOwner.IsOwner {
		t.Fatalf("detail = %#v", byOwner)
	}
	if byOwner.Location == nil || byOwner.Location.Name != "Kokand" {
		t.Fatalf("location = %#v, want Kokand", byOwner.Location)
	}
	if byOwner.BalanceUSD == nil || *byOwner.BalanceUSD != 7200 || byOwner.ProfitUZS == nil || *byOwner.ProfitUZS != 940000 {
		t.Fatalf("money = %v/%v, want 7200/940000", byOwner.BalanceUSD, byOwner.ProfitUZS)
	}
	if byOwner.Removal != (RemovalState{Allowed: false, BalanceIsZero: false, NoActiveOrders: true}) {
		t.Fatalf("removal = %#v", byOwner.Removal)
	}
	if byOwner.Permissions != (MemberPermissions{CanEditRole: true, CanEditLocation: true, CanAdjustBalance: true, CanRemove: true}) {
		t.Fatalf("owner permissions = %#v", byOwner.Permissions)
	}

	if got := f.detail(f.manager, f.aziz).Permissions; got != (MemberPermissions{CanEditLocation: true, CanAdjustBalance: true, CanRemove: true}) {
		t.Fatalf("manager permissions on employee = %#v", got)
	}
	if got := f.detail(f.investor, f.aziz).Permissions; got != (MemberPermissions{}) {
		t.Fatalf("investor permissions = %#v, want none", got)
	}
	if got := f.detail(f.aziz, f.aziz).Permissions; got != (MemberPermissions{}) {
		t.Fatalf("own permissions = %#v, want none", got)
	}
	investor := f.detail(f.manager, f.investor)
	if investor.BalanceUSD != nil || investor.ProfitUZS != nil || investor.Location != nil {
		t.Fatalf("investor detail = %#v, want no money or location", investor)
	}
	if investor.Permissions != (MemberPermissions{}) {
		t.Fatalf("manager permissions on investor = %#v, want none", investor.Permissions)
	}
	if got := f.detail(f.manager, f.owner).Permissions; got != (MemberPermissions{}) {
		t.Fatalf("permissions on owner = %#v, want none", got)
	}

	f.pendingOrder()
	if got := f.detail(f.owner, f.javohir).Removal; got != (RemovalState{BalanceIsZero: true, NoActiveOrders: false}) {
		t.Fatalf("removal with a pending order = %#v", got)
	}

	for _, tt := range []struct{ actor, target uuid.UUID }{{f.aziz, f.javohir}, {f.outsider, f.aziz}, {f.owner, f.outsider}} {
		if _, err := f.service.GetMember(ctx, tt.actor, f.groupID, tt.target); !errors.Is(err, apperror.ErrRecordNotFound) {
			t.Fatalf("get member %s as %s error = %v, want not found", tt.target, tt.actor, err)
		}
	}
}

func TestAdjustBalance_RecordsHistory(t *testing.T) {
	f := newMemberFixture(t)
	ctx := context.Background()
	addEmployeeBalance(t, f.pool, f.groupID, f.azizMember, 7200)

	up, err := f.service.AdjustBalance(ctx, f.manager, f.groupID, f.aziz, AdjustBalanceInput{NewBalanceUSD: 7500, Reason: "  Cash correction  "})
	if err != nil {
		t.Fatalf("adjust balance: %v", err)
	}
	if up.Direction != "increase" || up.AmountUSD != 300 || up.OldBalanceUSD != 7200 || up.NewBalanceUSD != 7500 || up.Reason != "Cash correction" || up.ChangedBy.Name != "manager" {
		t.Fatalf("adjustment = %#v", up)
	}
	down, err := f.service.AdjustBalance(ctx, f.owner, f.groupID, f.aziz, AdjustBalanceInput{NewBalanceUSD: -100})
	if err != nil {
		t.Fatalf("adjust balance below zero: %v", err)
	}
	if down.Direction != "decrease" || down.AmountUSD != -7600 || down.Reason != "" {
		t.Fatalf("adjustment = %#v", down)
	}
	// An employee without a balance row is adjusted from 0.
	if first, err := f.service.AdjustBalance(ctx, f.manager, f.groupID, f.javohir, AdjustBalanceInput{NewBalanceUSD: 50}); err != nil || first.OldBalanceUSD != 0 {
		t.Fatalf("first adjustment = %#v, %v", first, err)
	}

	history, err := f.service.ListBalanceAdjustments(ctx, f.investor, f.groupID, f.aziz, Page{})
	if err != nil {
		t.Fatalf("balance history: %v", err)
	}
	if history.CurrentBalanceUSD != -100 || history.Member.FullName != "aziz" || history.Member.LocationName != "Kokand" {
		t.Fatalf("history header = %#v", history)
	}
	if len(history.Adjustments) != 2 || history.Adjustments[0].ID != down.ID || history.Adjustments[1].ID != up.ID {
		t.Fatalf("history = %#v, want newest first", history.Adjustments)
	}
	page, err := f.service.ListBalanceAdjustments(ctx, f.aziz, f.groupID, f.aziz, Page{Limit: 1, Offset: 1})
	if err != nil || len(page.Adjustments) != 1 || page.Adjustments[0].ID != up.ID {
		t.Fatalf("own history page = %#v, %v", page.Adjustments, err)
	}

	tests := []struct {
		name    string
		actor   uuid.UUID
		target  uuid.UUID
		input   AdjustBalanceInput
		wantErr error
	}{
		{name: "same balance", actor: f.manager, target: f.aziz, input: AdjustBalanceInput{NewBalanceUSD: -100}, wantErr: apperror.ErrInvalidData},
		{name: "out of range", actor: f.manager, target: f.aziz, input: AdjustBalanceInput{NewBalanceUSD: maxBalanceUSD + 1}, wantErr: apperror.ErrInvalidData},
		{name: "reason too long", actor: f.manager, target: f.aziz, input: AdjustBalanceInput{NewBalanceUSD: 1, Reason: strings.Repeat("x", 501)}, wantErr: apperror.ErrInvalidData},
		{name: "investor", actor: f.investor, target: f.aziz, input: AdjustBalanceInput{NewBalanceUSD: 1}, wantErr: apperror.ErrForbidden},
		{name: "employee on self", actor: f.aziz, target: f.aziz, input: AdjustBalanceInput{NewBalanceUSD: 1}, wantErr: apperror.ErrForbidden},
		{name: "employee on another", actor: f.aziz, target: f.javohir, input: AdjustBalanceInput{NewBalanceUSD: 1}, wantErr: apperror.ErrRecordNotFound},
		{name: "target is not an employee", actor: f.owner, target: f.investor, input: AdjustBalanceInput{NewBalanceUSD: 1}, wantErr: apperror.ErrRecordNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := f.service.AdjustBalance(ctx, tt.actor, f.groupID, tt.target, tt.input); !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
		})
	}
	if _, err := f.service.ListBalanceAdjustments(ctx, f.javohir, f.groupID, f.aziz, Page{}); !errors.Is(err, apperror.ErrRecordNotFound) {
		t.Fatalf("other employee history error = %v, want not found", err)
	}
	if _, err := f.service.ListBalanceAdjustments(ctx, f.owner, f.groupID, f.aziz, Page{Limit: 101}); !errors.Is(err, apperror.ErrInvalidData) {
		t.Fatalf("oversized page error = %v, want invalid data", err)
	}
}

// An order completing while a manager adjusts the balance must not be lost.
func TestAdjustBalance_ConcurrentOrderEffectIsNotLost(t *testing.T) {
	f := newMemberFixture(t)
	addEmployeeBalance(t, f.pool, f.groupID, f.azizMember, 1000)

	var wg sync.WaitGroup
	var adjustment BalanceAdjustment
	var adjustErr, orderErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		adjustment, adjustErr = f.service.AdjustBalance(context.Background(), f.manager, f.groupID, f.aziz, AdjustBalanceInput{NewBalanceUSD: 5000})
	}()
	go func() {
		defer wg.Done()
		_, orderErr = f.pool.Exec(context.Background(), `
			UPDATE employee_balances SET balance_usd = balance_usd + 7000 WHERE member_id = $1
		`, f.azizMember)
	}()
	wg.Wait()
	if adjustErr != nil || orderErr != nil {
		t.Fatalf("adjust = %v, order = %v", adjustErr, orderErr)
	}
	final := *f.detail(f.owner, f.aziz).BalanceUSD
	switch adjustment.OldBalanceUSD {
	case 1000: // the adjustment ran first, then the order added to it
		if final != 12000 {
			t.Fatalf("final balance = %d, want 12000", final)
		}
	case 8000: // the order ran first and the adjustment saw it
		if final != 5000 {
			t.Fatalf("final balance = %d, want 5000", final)
		}
	default:
		t.Fatalf("adjustment old balance = %d, want 1000 or 8000", adjustment.OldBalanceUSD)
	}
}

func TestEditMember_Role(t *testing.T) {
	f := newMemberFixture(t)
	ctx := context.Background()
	manager, investor, employee, bogus := "manager", "investor", "employee", "boss"

	if _, err := f.service.EditMember(ctx, f.manager, f.groupID, f.aziz, EditMemberInput{Role: &investor}); !errors.Is(err, apperror.ErrForbidden) {
		t.Fatalf("manager changing a role error = %v, want forbidden", err)
	}
	if _, err := f.service.EditMember(ctx, f.owner, f.groupID, f.owner, EditMemberInput{Role: &investor}); !errors.Is(err, apperror.ErrConflict) {
		t.Fatalf("changing the owner's role error = %v, want conflict", err)
	}
	if _, err := f.service.EditMember(ctx, f.owner, f.groupID, f.aziz, EditMemberInput{Role: &bogus}); !errors.Is(err, apperror.ErrInvalidData) {
		t.Fatalf("unknown role error = %v, want invalid data", err)
	}
	if _, err := f.service.EditMember(ctx, f.owner, f.groupID, f.aziz, EditMemberInput{}); !errors.Is(err, apperror.ErrInvalidData) {
		t.Fatalf("empty edit error = %v, want invalid data", err)
	}
	if _, err := f.service.EditMember(ctx, f.aziz, f.groupID, f.aziz, EditMemberInput{Role: &employee}); !errors.Is(err, apperror.ErrForbidden) {
		t.Fatalf("employee editing themselves error = %v, want forbidden", err)
	}

	addEmployeeBalance(t, f.pool, f.groupID, f.azizMember, 10)
	if _, err := f.service.EditMember(ctx, f.owner, f.groupID, f.aziz, EditMemberInput{Role: &manager}); !errors.Is(err, apperror.ErrConflict) {
		t.Fatalf("role change with a balance error = %v, want conflict", err)
	}
	f.exec(`UPDATE employee_balances SET balance_usd = 0 WHERE member_id = $1`, f.azizMember)
	f.pendingOrder()
	if _, err := f.service.EditMember(ctx, f.owner, f.groupID, f.aziz, EditMemberInput{Role: &manager}); !errors.Is(err, apperror.ErrConflict) {
		t.Fatalf("role change with an active order error = %v, want conflict", err)
	}
	f.exec(`UPDATE orders SET status = 'cancelled' WHERE group_id = $1`, f.groupID)

	promoted, err := f.service.EditMember(ctx, f.owner, f.groupID, f.aziz, EditMemberInput{Role: &manager})
	if err != nil {
		t.Fatalf("promote employee: %v", err)
	}
	if promoted.Role != "manager" || promoted.Location != nil || promoted.BalanceUSD != nil {
		t.Fatalf("promoted member = %#v, want manager without location or balance", promoted)
	}
	var audits int
	if err := f.pool.QueryRow(ctx, `SELECT COUNT(*) FROM audit_logs WHERE action = 'member.role_changed' AND entity_id = $1`, f.azizMember).Scan(&audits); err != nil || audits != 1 {
		t.Fatalf("role audit logs = %d, %v; want 1", audits, err)
	}

	same, err := f.service.EditMember(ctx, f.manager, f.groupID, f.aziz, EditMemberInput{Role: &manager})
	if err != nil || same.Role != "manager" {
		t.Fatalf("unchanged role = %#v, %v; want a no-op", same.Role, err)
	}

	demoted, err := f.service.EditMember(ctx, f.owner, f.groupID, f.investor, EditMemberInput{Role: &employee})
	if err != nil {
		t.Fatalf("make investor an employee: %v", err)
	}
	if demoted.Role != "employee" || demoted.BalanceUSD == nil || *demoted.BalanceUSD != 0 {
		t.Fatalf("new employee = %#v, want a $0 balance", demoted)
	}
}

func TestEditMember_Location(t *testing.T) {
	f := newMemberFixture(t)
	ctx := context.Background()
	none := uuid.Nil

	moved, err := f.service.EditMember(ctx, f.manager, f.groupID, f.aziz, EditMemberInput{LocationID: &f.andijan})
	if err != nil {
		t.Fatalf("move employee: %v", err)
	}
	if moved.Location == nil || moved.Location.ID != f.andijan {
		t.Fatalf("location = %#v, want Andijan", moved.Location)
	}
	assigned, err := f.service.EditMember(ctx, f.manager, f.groupID, f.javohir, EditMemberInput{LocationID: &f.kokand})
	if err != nil || assigned.Location == nil || assigned.Location.ID != f.kokand {
		t.Fatalf("assign freed location = %#v, %v", assigned.Location, err)
	}
	if _, err := f.service.EditMember(ctx, f.manager, f.groupID, f.aziz, EditMemberInput{LocationID: &f.kokand}); !errors.Is(err, apperror.ErrConflict) {
		t.Fatalf("taken location error = %v, want conflict", err)
	}
	unassigned, err := f.service.EditMember(ctx, f.manager, f.groupID, f.aziz, EditMemberInput{LocationID: &none})
	if err != nil || unassigned.Location != nil {
		t.Fatalf("unassign = %#v, %v", unassigned.Location, err)
	}

	unknown := uuid.New()
	if _, err := f.service.EditMember(ctx, f.manager, f.groupID, f.aziz, EditMemberInput{LocationID: &unknown}); !errors.Is(err, apperror.ErrRecordNotFound) {
		t.Fatalf("unknown location error = %v, want not found", err)
	}
	if _, err := f.service.EditMember(ctx, f.manager, f.groupID, f.investor, EditMemberInput{LocationID: &f.andijan}); !errors.Is(err, apperror.ErrInvalidData) {
		t.Fatalf("location for an investor error = %v, want invalid data", err)
	}
	if _, err := f.service.EditMember(ctx, f.investor, f.groupID, f.aziz, EditMemberInput{LocationID: &f.andijan}); !errors.Is(err, apperror.ErrForbidden) {
		t.Fatalf("investor changing a location error = %v, want forbidden", err)
	}
}

func TestRemoveMember(t *testing.T) {
	f := newMemberFixture(t)
	ctx := context.Background()
	tests := []struct {
		name          string
		actor, target uuid.UUID
		wantErr       error
	}{
		{name: "owner cannot be removed", actor: f.manager, target: f.owner, wantErr: apperror.ErrConflict},
		{name: "cannot remove yourself", actor: f.manager, target: f.manager, wantErr: apperror.ErrConflict},
		{name: "manager cannot remove an investor", actor: f.manager, target: f.investor, wantErr: apperror.ErrForbidden},
		{name: "investor cannot remove", actor: f.investor, target: f.aziz, wantErr: apperror.ErrForbidden},
		{name: "employee cannot see the target", actor: f.javohir, target: f.aziz, wantErr: apperror.ErrRecordNotFound},
		{name: "unknown member", actor: f.owner, target: f.outsider, wantErr: apperror.ErrRecordNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := f.service.RemoveMember(ctx, tt.actor, f.groupID, tt.target); !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
		})
	}

	addEmployeeBalance(t, f.pool, f.groupID, f.azizMember, 7200)
	if err := f.service.RemoveMember(ctx, f.manager, f.groupID, f.aziz); !errors.Is(err, apperror.ErrConflict) {
		t.Fatalf("remove with a balance error = %v, want conflict", err)
	}
	f.exec(`UPDATE employee_balances SET balance_usd = 0 WHERE member_id = $1`, f.azizMember)
	f.pendingOrder()
	if err := f.service.RemoveMember(ctx, f.manager, f.groupID, f.aziz); !errors.Is(err, apperror.ErrConflict) {
		t.Fatalf("remove with an active order error = %v, want conflict", err)
	}
	f.exec(`UPDATE orders SET status = 'completed' WHERE group_id = $1`, f.groupID)

	if err := f.service.RemoveMember(ctx, f.manager, f.groupID, f.aziz); err != nil {
		t.Fatalf("remove employee: %v", err)
	}
	if _, err := f.service.GetMember(ctx, f.owner, f.groupID, f.aziz); !errors.Is(err, apperror.ErrRecordNotFound) {
		t.Fatalf("removed member error = %v, want not found", err)
	}
	var holder *uuid.UUID
	if err := f.pool.QueryRow(ctx, `SELECT employee_id FROM locations WHERE id = $1`, f.kokand).Scan(&holder); err != nil || holder != nil {
		t.Fatalf("location holder = %v, %v; want freed", holder, err)
	}
	var audits int
	if err := f.pool.QueryRow(ctx, `SELECT COUNT(*) FROM audit_logs WHERE action = 'member.removed' AND entity_id = $1`, f.azizMember).Scan(&audits); err != nil || audits != 1 {
		t.Fatalf("removal audit logs = %d, %v; want 1", audits, err)
	}
	if err := f.service.RemoveMember(ctx, f.owner, f.groupID, f.investor); err != nil {
		t.Fatalf("owner removes investor: %v", err)
	}

	// A removed user can be invited again and gets a fresh membership.
	invitation, err := f.service.Invite(ctx, f.owner, f.groupID, InviteInput{UserID: f.aziz, Role: "employee"})
	if err != nil {
		t.Fatalf("re-invite removed member: %v", err)
	}
	if _, err := f.service.RespondInvitation(ctx, f.aziz, invitation.ID, "accept"); err != nil {
		t.Fatalf("accept re-invitation: %v", err)
	}
	if rejoined := f.detail(f.owner, f.aziz); rejoined.MemberID == f.azizMember || rejoined.Location != nil {
		t.Fatalf("rejoined member = %#v, want a new membership without a location", rejoined)
	}
}
