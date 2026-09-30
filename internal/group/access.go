package group

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/qobulov/brothers-app/pkg/apperror"
)

func (s *Service) requireManager(ctx context.Context, actorID, groupID uuid.UUID) error {
	var allowed bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM group_members
			JOIN groups ON groups.id = group_members.group_id
			WHERE group_members.group_id = $1
			  AND group_members.user_id = $2
			  AND group_members.deleted_at IS NULL
			  AND groups.deleted_at IS NULL
			  AND groups.is_active
			  AND (group_members.is_owner OR group_members.role::text IN ('owner', 'admin', 'manager'))
		)
	`, groupID, actorID).Scan(&allowed)
	if err != nil {
		return fmt.Errorf("checking group manager permission: %w", err)
	}
	if !allowed {
		return apperror.ErrForbidden
	}
	return nil
}

func (s *Service) requireMember(ctx context.Context, actorID, groupID uuid.UUID) error {
	var exists bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM group_members members
			JOIN groups ON groups.id = members.group_id
			WHERE members.group_id = $1
			  AND members.user_id = $2
			  AND members.deleted_at IS NULL
			  AND groups.deleted_at IS NULL
			  AND groups.is_active
		)
	`, groupID, actorID).Scan(&exists)
	if err != nil {
		return fmt.Errorf("checking group membership: %w", err)
	}
	if !exists {
		return apperror.ErrRecordNotFound
	}
	return nil
}
