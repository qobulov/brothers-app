package group

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qobulov/brothers-app/pkg/apperror"
	"github.com/qobulov/brothers-app/pkg/helpers"
)

type LocationEmployee struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	AvatarURL string    `json:"avatar_url"`
}

type Location struct {
	ID        uuid.UUID         `json:"id"`
	GroupID   uuid.UUID         `json:"group_id"`
	Name      string            `json:"name"`
	Employee  *LocationEmployee `json:"employee,omitempty"`
	CreatedBy uuid.UUID         `json:"created_by"`
}

type Customer struct {
	ID    uuid.UUID `json:"id"`
	Phone string    `json:"phone"`
}

type CreateLocationInput struct {
	Name       string
	EmployeeID uuid.UUID
}

func (s *Service) ListLocations(ctx context.Context, actorID, groupID uuid.UUID) ([]Location, error) {
	if err := s.requireMember(ctx, actorID, groupID); err != nil {
		return nil, err
	}

	rows, err := s.pool.Query(ctx, `
		SELECT locations.id, locations.group_id, locations.name,
		       users.id,
		       COALESCE(
		           NULLIF(btrim(concat_ws(' ', users.first_name, users.last_name)), ''),
		           NULLIF(users.name, ''), NULLIF(users.username, ''), ''
		       ) AS employee_name,
		       COALESCE(users.avatar_url, ''), locations.created_by
		FROM locations
		LEFT JOIN group_members members
		  ON members.group_id = locations.group_id
		 AND members.id = locations.employee_id
		 AND members.deleted_at IS NULL
		LEFT JOIN users
		  ON users.id = members.user_id
		 AND users.deleted_at IS NULL
		WHERE locations.group_id = $1
		  AND locations.deleted_at IS NULL
		  AND EXISTS (
		      SELECT 1
		      FROM group_members actor_membership
		      JOIN groups ON groups.id = actor_membership.group_id
		      WHERE actor_membership.group_id = locations.group_id
		        AND actor_membership.user_id = $2
		        AND actor_membership.deleted_at IS NULL
		        AND groups.deleted_at IS NULL
		        AND groups.is_active
		  )
		ORDER BY locations.created_at ASC, locations.id ASC
	`, groupID, actorID)
	if err != nil {
		return nil, fmt.Errorf("listing group locations: %w", err)
	}
	defer rows.Close()

	locations := make([]Location, 0)
	for rows.Next() {
		location, err := scanLocation(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning group location: %w", err)
		}
		locations = append(locations, location)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating group locations: %w", err)
	}
	return locations, nil
}

func (s *Service) CreateLocation(ctx context.Context, actorID, groupID uuid.UUID, input CreateLocationInput) (Location, error) {
	if err := s.requireManager(ctx, actorID, groupID); err != nil {
		return Location{}, err
	}

	name, err := validLocationName(input.Name)
	if err != nil {
		return Location{}, err
	}
	employeeMemberID, err := s.resolveEmployeeMemberID(ctx, groupID, input.EmployeeID)
	if err != nil {
		return Location{}, err
	}

	locationID := uuid.New()
	now := s.now().UTC()
	_, err = s.pool.Exec(ctx, `
		INSERT INTO locations (
			id, group_id, name, employee_id, created_by, created_at, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $6)
	`, locationID, groupID, name, employeeMemberID, actorID, now)
	if err != nil {
		return Location{}, fmt.Errorf("creating group location: %w", err)
	}

	return s.getLocation(ctx, groupID, locationID)
}

func (s *Service) DeleteLocation(ctx context.Context, actorID, groupID, locationID uuid.UUID) error {
	if err := s.requireManager(ctx, actorID, groupID); err != nil {
		return err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning location deletion: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var employeeMemberID *uuid.UUID
	err = tx.QueryRow(ctx, `
		SELECT employee_id
		FROM locations
		WHERE id = $1 AND group_id = $2 AND deleted_at IS NULL
		FOR UPDATE
	`, locationID, groupID).Scan(&employeeMemberID)
	if errors.Is(err, pgx.ErrNoRows) {
		return apperror.ErrRecordNotFound
	}
	if err != nil {
		return fmt.Errorf("locking group location: %w", err)
	}
	if employeeMemberID != nil {
		return apperror.New(apperror.ErrConflict, apperror.Text{UZ: "Joyga xodim biriktirilgan, avval uni boshqa joyga o'tkazing", RU: "К локации привязан сотрудник, сначала переведите его", EN: "The location has an assigned employee; move them first"})
	}

	now := s.now().UTC()
	_, err = tx.Exec(ctx, `
		UPDATE locations
		SET deleted_at = $3, updated_at = $3
		WHERE id = $1 AND group_id = $2 AND deleted_at IS NULL
	`, locationID, groupID, now)
	if err != nil {
		return fmt.Errorf("deleting group location: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing location deletion: %w", err)
	}
	return nil
}

func (s *Service) ListCustomers(ctx context.Context, actorID, groupID uuid.UUID, query string) ([]Customer, error) {
	if err := s.requireMember(ctx, actorID, groupID); err != nil {
		return nil, err
	}
	query = strings.TrimSpace(query)
	if len(query) > 20 {
		return nil, apperror.ErrInvalidData
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, phone
		FROM customers
		WHERE group_id = $1
		  AND deleted_at IS NULL
		  AND ($2::text = '' OR phone ILIKE '%' || $2 || '%')
		  AND EXISTS (
		      SELECT 1
		      FROM group_members actor_membership
		      JOIN groups ON groups.id = actor_membership.group_id
		      WHERE actor_membership.group_id = customers.group_id
		        AND actor_membership.user_id = $3
		        AND actor_membership.deleted_at IS NULL
		        AND groups.deleted_at IS NULL
		        AND groups.is_active
		  )
		ORDER BY created_at DESC, id DESC
	`, groupID, helpers.EscapeLike(query), actorID)
	if err != nil {
		return nil, fmt.Errorf("listing group customers: %w", err)
	}
	defer rows.Close()

	customers := make([]Customer, 0)
	for rows.Next() {
		var customer Customer
		if err := rows.Scan(&customer.ID, &customer.Phone); err != nil {
			return nil, fmt.Errorf("scanning group customer: %w", err)
		}
		customers = append(customers, customer)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating group customers: %w", err)
	}
	return customers, nil
}

func (s *Service) getLocation(ctx context.Context, groupID, locationID uuid.UUID) (Location, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT locations.id, locations.group_id, locations.name,
		       users.id,
		       COALESCE(
		           NULLIF(btrim(concat_ws(' ', users.first_name, users.last_name)), ''),
		           NULLIF(users.name, ''), NULLIF(users.username, ''), ''
		       ) AS employee_name,
		       COALESCE(users.avatar_url, ''), locations.created_by
		FROM locations
		LEFT JOIN group_members members
		  ON members.group_id = locations.group_id
		 AND members.id = locations.employee_id
		 AND members.deleted_at IS NULL
		LEFT JOIN users
		  ON users.id = members.user_id
		 AND users.deleted_at IS NULL
		WHERE locations.id = $1
		  AND locations.group_id = $2
		  AND locations.deleted_at IS NULL
	`, locationID, groupID)
	location, err := scanLocation(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Location{}, apperror.ErrRecordNotFound
	}
	if err != nil {
		return Location{}, fmt.Errorf("getting group location: %w", err)
	}
	return location, nil
}

type locationScanner interface {
	Scan(dest ...any) error
}

func scanLocation(row locationScanner) (Location, error) {
	var location Location
	var employeeID *uuid.UUID
	var employeeName, employeeAvatar string
	if err := row.Scan(
		&location.ID, &location.GroupID, &location.Name,
		&employeeID, &employeeName, &employeeAvatar, &location.CreatedBy,
	); err != nil {
		return Location{}, err
	}
	if employeeID != nil {
		location.Employee = &LocationEmployee{ID: *employeeID, Name: employeeName, AvatarURL: employeeAvatar}
	}
	return location, nil
}

func (s *Service) resolveEmployeeMemberID(ctx context.Context, groupID, employeeUserID uuid.UUID) (*uuid.UUID, error) {
	if employeeUserID == uuid.Nil {
		return nil, nil
	}
	var memberID uuid.UUID
	err := s.pool.QueryRow(ctx, `
		SELECT id
		FROM group_members
		WHERE group_id = $1
		  AND user_id = $2
		  AND role::text = 'employee'
		  AND deleted_at IS NULL
	`, groupID, employeeUserID).Scan(&memberID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperror.ErrRecordNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("resolving location employee: %w", err)
	}
	return &memberID, nil
}

func validLocationName(value string) (string, error) {
	name := strings.TrimSpace(value)
	if name == "" || len(name) > 255 {
		return "", apperror.ErrInvalidData
	}
	return name, nil
}
