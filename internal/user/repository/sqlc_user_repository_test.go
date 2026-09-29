package repository_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	db "github.com/qobulov/brothers-app/internal/db"
	"github.com/qobulov/brothers-app/internal/user/repository"
	"github.com/qobulov/brothers-app/pkg/database"
	"github.com/stretchr/testify/suite"
)

type UserRepositoryTestSuite struct {
	suite.Suite
	db      *pgxpool.Pool
	repo    repository.UserRepository
	cleanup func()
}

func (s *UserRepositoryTestSuite) SetupTest() {
	s.db, s.cleanup = database.SetupTestDB(s.T())
	s.repo = repository.NewSQLCUserRepository(db.New(s.db))
}

func (s *UserRepositoryTestSuite) TearDownTest() {
	if s.cleanup != nil {
		s.cleanup()
	}
}

func TestUserRepositoryTestSuite(t *testing.T) {
	suite.Run(t, new(UserRepositoryTestSuite))
}

func (s *UserRepositoryTestSuite) createUser(username, email string, active bool) uuid.UUID {
	s.T().Helper()
	id := uuid.New()
	_, err := s.db.Exec(s.T().Context(), `
		INSERT INTO users (id, username, email, language, is_active, created_at, updated_at)
		VALUES ($1, $2, $3, 'uz', $4, now(), now())
	`, id, username, email, active)
	s.Require().NoError(err)
	return id
}

func (s *UserRepositoryTestSuite) TestSearch_MatchesUsernameOrEmailCaseInsensitively() {
	byUsername := s.createUser("JohnDoe", "first@example.com", true)
	byEmail := s.createUser("someone", "johnny@example.com", true)
	s.createUser("other", "other@example.com", true)

	users, err := s.repo.Search(s.T().Context(), "john")
	s.Require().NoError(err)
	s.Require().Len(users, 2)
	s.Equal(byUsername, users[0].ID)
	s.Equal(byEmail, users[1].ID)
	s.Equal("JohnDoe", users[0].Username)
	s.Equal("first@example.com", users[0].Email)
}

func (s *UserRepositoryTestSuite) TestSearch_ExcludesInactiveAndDeletedUsers() {
	s.createUser("john_inactive", "inactive@example.com", false)
	deleted := s.createUser("john_deleted", "deleted@example.com", true)
	_, err := s.db.Exec(s.T().Context(), `UPDATE users SET deleted_at = now() WHERE id = $1`, deleted)
	s.Require().NoError(err)

	users, err := s.repo.Search(s.T().Context(), "john")
	s.NoError(err)
	s.Empty(users)
}

func (s *UserRepositoryTestSuite) TestSearch_TreatsWildcardsLiterally() {
	literal := s.createUser("a_b", "ab1@example.com", true)
	s.createUser("axb", "ab2@example.com", true)

	users, err := s.repo.Search(s.T().Context(), "a_b")
	s.Require().NoError(err)
	s.Require().Len(users, 1)
	s.Equal(literal, users[0].ID)

	users, err = s.repo.Search(s.T().Context(), "%")
	s.NoError(err)
	s.Empty(users)
}

func (s *UserRepositoryTestSuite) TestSearch_CancelledContext() {
	ctx, cancel := context.WithCancel(s.T().Context())
	cancel()
	_, err := s.repo.Search(ctx, "john")
	s.ErrorIs(err, context.Canceled)
}
