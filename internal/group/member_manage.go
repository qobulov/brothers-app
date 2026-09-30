package group

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qobulov/brothers-app/pkg/apperror"
)

// EditMemberInput changes only the non-nil fields. A LocationID of uuid.Nil
// unassigns the member's location.
type EditMemberInput struct {
	Role       *string
	LocationID *uuid.UUID
}

// EditMember changes a member's role (owner only) and location (any manager).
func (s *Service) EditMember(ctx context.Context, actorID, groupID, userID uuid.UUID, input EditMemberInput) (MemberDetail, error) {
	if input.Role == nil && input.LocationID == nil {
		return MemberDetail{}, apperror.NothingToUpdate()
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return MemberDetail{}, fmt.Errorf("beginning member edit: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	edit, err := newMemberChange(ctx, tx, memberLookup{groupID: groupID, userID: userID, lock: true}, actorID)
	if err != nil {
		return MemberDetail{}, err
	}
	if !edit.actor.isManager() {
		return MemberDetail{}, apperror.ErrForbidden
	}
	edit.at = s.now().UTC()
	if input.Role != nil {
		if err := edit.changeRole(ctx, tx, *input.Role); err != nil {
			return MemberDetail{}, err
		}
	}
	if input.LocationID != nil {
		if err := edit.changeLocation(ctx, tx, *input.LocationID); err != nil {
			return MemberDetail{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return MemberDetail{}, fmt.Errorf("committing member edit: %w", err)
	}
	return s.GetMember(ctx, actorID, groupID, userID)
}

// RemoveMember soft-deletes a membership once nothing is left unsettled.
func (s *Service) RemoveMember(ctx context.Context, actorID, groupID, userID uuid.UUID) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning member removal: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	removal, err := newMemberChange(ctx, tx, memberLookup{groupID: groupID, userID: userID, lock: true}, actorID)
	if err != nil {
		return err
	}
	removal.at = s.now().UTC()
	target := removal.target
	switch {
	case target.isOwner:
		return apperror.New(apperror.ErrConflict, apperror.Text{UZ: "Guruh egasini chiqarib bo'lmaydi", RU: "Владельца группы нельзя удалить", EN: "The group owner cannot be removed"})
	case target.memberID == removal.actor.memberID:
		return apperror.New(apperror.ErrConflict, apperror.Text{UZ: "O'zingizni guruhdan chiqara olmaysiz", RU: "Нельзя удалить себя из группы", EN: "You cannot remove yourself"})
	case !removal.actor.canRemove(target):
		return apperror.ErrForbidden
	}
	if err := target.requireSettled(); err != nil {
		return err
	}
	if err := removal.unassignLocation(ctx, tx); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		UPDATE group_members SET deleted_at = $2, updated_at = $2 WHERE id = $1
	`, target.memberID, removal.at)
	if err != nil {
		return fmt.Errorf("removing group member: %w", err)
	}
	if err := removal.audit(ctx, tx, "member.removed", map[string]any{"role": target.role}, map[string]any{}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing member removal: %w", err)
	}
	return nil
}

// memberChange is one manager action on one locked member.
type memberChange struct {
	groupID uuid.UUID
	actor   actor
	target  memberRow
	at      time.Time
}

func newMemberChange(ctx context.Context, q querier, lookup memberLookup, actorID uuid.UUID) (memberChange, error) {
	a, err := loadActor(ctx, q, lookup.groupID, actorID)
	if err != nil {
		return memberChange{}, err
	}
	target, err := loadMember(ctx, q, lookup)
	if err != nil {
		return memberChange{}, err
	}
	if !a.canView(target) {
		return memberChange{}, apperror.ErrRecordNotFound
	}
	return memberChange{groupID: lookup.groupID, actor: a, target: target}, nil
}

// requireSettled blocks leaving the group or the employee role while the
// member still holds money or takes part in an unfinished order.
func (m memberRow) requireSettled() error {
	if m.balanceUSD != 0 {
		balance := apperror.FormatNumber(m.balanceUSD)
		return apperror.New(apperror.ErrConflict, apperror.Text{
			UZ: fmt.Sprintf("A'zoning balansi 0 bo'lishi kerak, hozir $%s", balance.UZ),
			RU: fmt.Sprintf("Баланс участника должен быть 0, сейчас $%s", balance.RU),
			EN: fmt.Sprintf("The member's balance must be 0; it is $%s", balance.EN),
		})
	}
	if m.activeOrders != 0 {
		return apperror.New(apperror.ErrConflict, apperror.Text{
			UZ: fmt.Sprintf("A'zoda %d ta faol buyurtma bor", m.activeOrders),
			RU: fmt.Sprintf("У участника активных заказов: %d", m.activeOrders),
			EN: fmt.Sprintf("The member has %d active orders", m.activeOrders),
		})
	}
	return nil
}

func (c *memberChange) changeRole(ctx context.Context, q querier, value string) error {
	role, err := invitationRole(value)
	if err != nil {
		return err
	}
	if role == c.target.role {
		return nil
	}
	if !c.actor.isOwner {
		return apperror.ErrForbidden
	}
	if c.target.isOwner {
		return apperror.New(apperror.ErrConflict, apperror.Text{UZ: "Guruh egasining rolini o'zgartirib bo'lmaydi", RU: "Роль владельца группы нельзя изменить", EN: "The group owner's role cannot change"})
	}
	if c.target.role == roleEmployee {
		if err := c.target.requireSettled(); err != nil {
			return err
		}
		if err := c.unassignLocation(ctx, q); err != nil {
			return err
		}
	}
	_, err = q.Exec(ctx, `
		UPDATE group_members SET role = $2::user_role, updated_at = $3 WHERE id = $1
	`, c.target.memberID, role, c.at)
	if err != nil {
		return fmt.Errorf("changing member role: %w", err)
	}
	if role == roleEmployee {
		if err := ensureBalanceRow(ctx, q, c.groupID, c.target.memberID); err != nil {
			return err
		}
	}
	oldRole := c.target.role
	c.target.role = role
	return c.audit(ctx, q, "member.role_changed", map[string]any{"role": oldRole}, map[string]any{"role": role})
}

func (c *memberChange) changeLocation(ctx context.Context, q querier, locationID uuid.UUID) error {
	if c.target.role != roleEmployee {
		return apperror.New(apperror.ErrInvalidData, apperror.Text{UZ: "Joy faqat xodimlarga biriktiriladi", RU: "Локация назначается только сотрудникам", EN: "Only employees have a location"})
	}
	if locationID == uuid.Nil {
		return c.unassignLocation(ctx, q)
	}
	if c.target.location != nil && c.target.location.ID == locationID {
		return nil
	}
	var holder *uuid.UUID
	err := q.QueryRow(ctx, `
		SELECT employee_id FROM locations
		WHERE id = $1 AND group_id = $2 AND deleted_at IS NULL
		FOR UPDATE
	`, locationID, c.groupID).Scan(&holder)
	if errors.Is(err, pgx.ErrNoRows) {
		return apperror.New(apperror.ErrRecordNotFound, apperror.Text{UZ: "Joy topilmadi", RU: "Локация не найдена", EN: "Location not found"})
	}
	if err != nil {
		return fmt.Errorf("locking location: %w", err)
	}
	if holder != nil {
		return apperror.New(apperror.ErrConflict, apperror.Text{UZ: "Bu joy boshqa xodimga biriktirilgan", RU: "Эта локация закреплена за другим сотрудником", EN: "This location is assigned to another employee"})
	}
	// An employee holds one location, so the old one is released first.
	if err := c.unassignLocation(ctx, q); err != nil {
		return err
	}
	_, err = q.Exec(ctx, `
		UPDATE locations SET employee_id = $2, updated_at = $3 WHERE id = $1
	`, locationID, c.target.memberID, c.at)
	if err != nil {
		return fmt.Errorf("assigning location: %w", err)
	}
	return nil
}

func (c *memberChange) unassignLocation(ctx context.Context, q querier) error {
	_, err := q.Exec(ctx, `
		UPDATE locations SET employee_id = NULL, updated_at = $3
		WHERE group_id = $1 AND employee_id = $2 AND deleted_at IS NULL
	`, c.groupID, c.target.memberID, c.at)
	if err != nil {
		return fmt.Errorf("unassigning location: %w", err)
	}
	c.target.location = nil
	return nil
}

func (c *memberChange) audit(ctx context.Context, q querier, action string, oldData, newData map[string]any) error {
	_, err := q.Exec(ctx, `
		INSERT INTO audit_logs (
			group_id, actor_user_id, action, entity_type, entity_id, old_data, new_data, created_at, updated_at
		)
		VALUES ($1, $2, $3, 'group_member', $4, $5, $6, $7, $7)
	`, c.groupID, c.actor.userID, action, c.target.memberID, oldData, newData, c.at)
	if err != nil {
		return fmt.Errorf("writing %s audit log: %w", action, err)
	}
	return nil
}

func ensureBalanceRow(ctx context.Context, q querier, groupID, memberID uuid.UUID) error {
	_, err := q.Exec(ctx, `
		INSERT INTO employee_balances (group_id, member_id, balance_usd)
		VALUES ($1, $2, 0)
		ON CONFLICT (group_id, member_id) WHERE deleted_at IS NULL DO NOTHING
	`, groupID, memberID)
	if err != nil {
		return fmt.Errorf("creating employee balance: %w", err)
	}
	return nil
}
