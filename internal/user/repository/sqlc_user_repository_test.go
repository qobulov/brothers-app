package repository_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	db "github.com/qobulov/brothers-app/internal/db"
	"github.com/qobulov/brothers-app/internal/entities"
	"github.com/qobulov/brothers-app/internal/user/repository"
	"github.com/qobulov/brothers-app/pkg/apperror"
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

func (s *UserRepositoryTestSuite) createUser(name string) *entities.User {
	s.T().Helper()
	id := uuid.New()
	username := "user_" + id.String()
	phone := "+998" + id.String()[:9]
	_, err := s.db.Exec(s.T().Context(), `
		INSERT INTO users (id, name, phone, username, language, is_active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, 'uz', true, now(), now())
	`, id, name, phone, username)
	s.Require().NoError(err)
	return &entities.User{ID: id, Name: name, Phone: phone, Username: username}
}

func (s *UserRepositoryTestSuite) TestFindByID() {
	user := s.createUser("Find By ID User")
	found, err := s.repo.FindByID(user.ID.String())
	s.NoError(err)
	s.Equal(user.ID, found.ID)
	s.Equal(user.Name, found.Name)
	s.Equal(user.Phone, found.Phone)
}

func (s *UserRepositoryTestSuite) TestFindByID_NotFound() {
	found, err := s.repo.FindByID(uuid.NewString())
	s.ErrorIs(err, apperror.ErrRecordNotFound)
	s.Nil(found)
}

func (s *UserRepositoryTestSuite) TestFindAll() {
	s.createUser("User 1")
	s.createUser("User 2")
	s.createUser("User 3")

	users, err := s.repo.FindAll()
	s.NoError(err)
	s.Len(users, 3)
}

func (s *UserRepositoryTestSuite) TestFindAll_Empty() {
	users, err := s.repo.FindAll()
	s.NoError(err)
	s.Empty(users)
}

func (s *UserRepositoryTestSuite) TestPatch() {
	user := s.createUser("Original Name")
	err := s.repo.Patch(user.ID.String(), &entities.User{Name: "Updated Name"})
	s.NoError(err)

	updated, err := s.repo.FindByID(user.ID.String())
	s.NoError(err)
	s.Equal("Updated Name", updated.Name)
	s.Equal(user.Phone, updated.Phone)
}

func (s *UserRepositoryTestSuite) TestPatch_NotFound() {
	err := s.repo.Patch(uuid.NewString(), &entities.User{Name: "Updated Name"})
	s.ErrorIs(err, apperror.ErrRecordNotFound)
}

func (s *UserRepositoryTestSuite) TestDelete() {
	user := s.createUser("Delete User")
	s.NoError(s.repo.Delete(user.ID.String()))

	found, err := s.repo.FindByID(user.ID.String())
	s.ErrorIs(err, apperror.ErrRecordNotFound)
	s.Nil(found)
}

func (s *UserRepositoryTestSuite) TestDelete_NotFound() {
	err := s.repo.Delete(uuid.NewString())
	s.ErrorIs(err, apperror.ErrRecordNotFound)
}
