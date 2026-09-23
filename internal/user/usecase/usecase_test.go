package usecase_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	db "github.com/qobulov/brothers-app/internal/db"
	"github.com/qobulov/brothers-app/internal/entities"
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

func (s *UserUseCaseTestSuite) createUser(name string) *entities.User {
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

func (s *UserUseCaseTestSuite) TestFindUserByID() {
	user := s.createUser("Find By ID User")
	found, err := s.service.FindUserByID(user.ID.String())
	s.NoError(err)
	s.Equal(user.ID, found.ID)
	s.Equal(user.Name, found.Name)
}

func (s *UserUseCaseTestSuite) TestFindAllUsers() {
	s.createUser("User 1")
	s.createUser("User 2")
	s.createUser("User 3")

	users, err := s.service.FindAllUsers()
	s.NoError(err)
	s.Len(users, 3)
}

func (s *UserUseCaseTestSuite) TestPatchUser() {
	user := s.createUser("Original Name")
	updated, err := s.service.PatchUser(user.ID.String(), &entities.User{Name: "Updated Name"})
	s.NoError(err)
	s.Equal("Updated Name", updated.Name)
	s.Equal(user.Phone, updated.Phone)
}

func (s *UserUseCaseTestSuite) TestDeleteUser() {
	user := s.createUser("Delete User")
	s.NoError(s.service.DeleteUser(user.ID.String()))

	found, err := s.service.FindUserByID(user.ID.String())
	s.Error(err)
	s.Nil(found)
}
