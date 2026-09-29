package usecase_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	db "github.com/qobulov/brothers-app/internal/db"
	"github.com/qobulov/brothers-app/internal/user/repository"
	"github.com/qobulov/brothers-app/internal/user/usecase"
	"github.com/qobulov/brothers-app/pkg/database"
	"github.com/stretchr/testify/suite"
)

type UserUseCaseTestSuite struct {
	suite.Suite
	db      *pgxpool.Pool
	service usecase.UserUseCase
	cleanup func()
}

func (s *UserUseCaseTestSuite) SetupTest() {
	s.db, s.cleanup = database.SetupTestDB(s.T())
	repo := repository.NewSQLCUserRepository(db.New(s.db))
	s.service = usecase.NewUserService(repo)
}

func (s *UserUseCaseTestSuite) TearDownTest() {
	if s.cleanup != nil {
		s.cleanup()
	}
}

func TestUserUseCaseTestSuite(t *testing.T) {
	suite.Run(t, new(UserUseCaseTestSuite))
}

func (s *UserUseCaseTestSuite) TestSearchUsers_TrimsQuery() {
	id := uuid.New()
	_, err := s.db.Exec(s.T().Context(), `
		INSERT INTO users (id, username, email, language, is_active, created_at, updated_at)
		VALUES ($1, 'searchable', 'searchable@example.com', 'uz', true, now(), now())
	`, id)
	s.Require().NoError(err)

	users, err := s.service.SearchUsers(s.T().Context(), "  search  ")
	s.Require().NoError(err)
	s.Require().Len(users, 1)
	s.Equal(id, users[0].ID)
}
