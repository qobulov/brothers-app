package routes_test

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/suite"
	"golang.org/x/crypto/bcrypt"

	"github.com/qobulov/brothers-app/internal/app"
	"github.com/qobulov/brothers-app/pkg/config"
	"github.com/qobulov/brothers-app/pkg/database"
)

type PublicRoutesTestSuite struct {
	suite.Suite
	db      *pgxpool.Pool
	app     *fiber.App
	cfg     *config.Config
	cleanup func()
}

func (s *PublicRoutesTestSuite) SetupTest() {
	// Setup test database with cleanup
	s.db, s.cleanup = database.SetupTestDB(s.T())

	// Load config for dev environment
	s.cfg = config.LoadConfig("dev")

	// Setup REST server with test database (For registering routes and middleware)
	var err error
	s.app, err = app.SetupRestServer(s.db, nil, s.cfg)
	s.NoError(err, "Failed to setup REST server")
}

func (s *PublicRoutesTestSuite) TearDownTest() {
	// Clean up database after each test
	if s.cleanup != nil {
		s.cleanup()
	}
}

func TestPublicRoutesTestSuite(t *testing.T) {
	suite.Run(t, new(PublicRoutesTestSuite))
}

// === USER ROUTES ===

func (s *PublicRoutesTestSuite) TestGetUsers() {
	req := httptest.NewRequest("GET", "/api/v1/users", nil)
	resp, err := s.app.Test(req, -1)
	s.NoError(err)
	s.Equal(fiber.StatusOK, resp.StatusCode)
}

func (s *PublicRoutesTestSuite) TestGetUserByID_NotFound() {
	req := httptest.NewRequest("GET", "/api/v1/users/9a176ca5-f3e0-4994-869c-fac0e8c9d5dc", nil)
	resp, err := s.app.Test(req, -1)
	s.NoError(err)
	s.NotEqual(fiber.StatusInternalServerError, resp.StatusCode)
}

// === AUTH ROUTES ===

func (s *PublicRoutesTestSuite) TestLogin() {
	const password = "securepassword123"
	s.createLoginUser("loginuser", "+998901234567", password)

	tests := []struct {
		name  string
		login string
	}{
		{name: "username", login: "loginuser"},
		{name: "phone", login: "+998 90 123 45 67"},
	}
	for _, test := range tests {
		s.Run(test.name, func() {
			body := map[string]string{"login": test.login, "password": password}
			jsonBody, err := json.Marshal(body)
			s.Require().NoError(err)

			req := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewBuffer(jsonBody))
			req.Header.Set("Content-Type", "application/json")
			resp, err := s.app.Test(req, -1)
			s.Require().NoError(err)
			defer resp.Body.Close()
			s.Equal(fiber.StatusOK, resp.StatusCode)
		})
	}
}

func (s *PublicRoutesTestSuite) TestLogin_InvalidCredentials() {
	body := map[string]string{
		"login":    "nonexistent-user",
		"password": "wrongpassword",
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.app.Test(req, -1)
	s.NoError(err)
	s.Equal(fiber.StatusUnauthorized, resp.StatusCode)
}

func (s *PublicRoutesTestSuite) TestLegacyAuthRoutesNotRegistered() {
	legacyPaths := map[string]bool{
		"/api/v1/auth/signin":            false,
		"/api/v1/auth/signup":            false,
		"/api/v1/auth/otp/verify":        false,
		"/api/v1/auth/register/resend":   false,
		"/api/v1/auth/password/resend":   false,
		"/api/v1/me/phone-change/resend": false,
	}
	for _, route := range s.app.GetRoutes() {
		if route.Method == fiber.MethodPost {
			if _, legacy := legacyPaths[route.Path]; legacy {
				legacyPaths[route.Path] = true
			}
		}
	}
	for path, registered := range legacyPaths {
		if registered {
			s.T().Errorf("legacy route %s is still registered", path)
		}
	}
}

func (s *PublicRoutesTestSuite) TestRegistrationRoutesRegistered() {
	wanted := map[string]bool{
		"/api/v1/auth/otp/send": false,
		"/api/v1/auth/register": false,
	}
	for _, route := range s.app.GetRoutes() {
		if route.Method == fiber.MethodPost {
			if _, ok := wanted[route.Path]; ok {
				wanted[route.Path] = true
			}
		}
	}
	for path, registered := range wanted {
		if !registered {
			s.T().Errorf("registration route %s is missing", path)
		}
	}
}

func (s *PublicRoutesTestSuite) TestRegisterConflictLocalized() {
	s.createLoginUser("existing-user", "+998901234577", "securepassword123")

	tests := []struct {
		name     string
		language string
		want     string
	}{
		{name: "uzbek", language: "uz", want: "Telefon raqami yoki foydalanuvchi nomi allaqachon mavjud"},
		{name: "russian", language: "ru", want: "Номер телефона или имя пользователя уже существует"},
		{name: "english", language: "en", want: "Phone or username already exists"},
	}

	for _, test := range tests {
		s.Run(test.name, func() {
			body, err := json.Marshal(map[string]string{
				"phone":      "+998901234577",
				"username":   "new-" + test.language,
				"first_name": "Qobul",
				"last_name":  "Qobulov",
				"password":   "strong-password",
				"language":   test.language,
				"otp_code":   "111111",
			})
			s.Require().NoError(err)

			req := httptest.NewRequest("POST", "/api/v1/auth/register", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			resp, err := s.app.Test(req, -1)
			s.Require().NoError(err)
			defer resp.Body.Close()
			s.Require().Equal(fiber.StatusConflict, resp.StatusCode)

			var envelope struct {
				Code    int    `json:"code"`
				Slug    string `json:"slug"`
				Message string `json:"message"`
			}
			s.Require().NoError(json.NewDecoder(resp.Body).Decode(&envelope))
			s.Equal(1409, envelope.Code)
			s.Equal("phone_or_username_exists", envelope.Slug)
			s.Equal(test.want, envelope.Message)
		})
	}
}

func (s *PublicRoutesTestSuite) TestCurrentProfilePatch() {
	const password = "securepassword123"
	s.createLoginUser("profileuser", "+998901234568", password)

	loginBody, err := json.Marshal(map[string]string{"login": "profileuser", "password": password})
	s.Require().NoError(err)
	loginRequest := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewBuffer(loginBody))
	loginRequest.Header.Set("Content-Type", "application/json")
	loginResponse, err := s.app.Test(loginRequest, -1)
	s.Require().NoError(err)
	defer loginResponse.Body.Close()
	s.Require().Equal(fiber.StatusOK, loginResponse.StatusCode)

	var loginEnvelope struct {
		Data struct {
			Tokens struct {
				AccessToken      string `json:"access_token"`
				AccessExpiresAt  string `json:"access_expires_at"`
				RefreshToken     string `json:"refresh_token"`
				RefreshExpiresAt string `json:"refresh_expires_at"`
			} `json:"tokens"`
		} `json:"data"`
	}
	s.Require().NoError(json.NewDecoder(loginResponse.Body).Decode(&loginEnvelope))
	s.Require().NotEmpty(loginEnvelope.Data.Tokens.AccessToken)
	s.Require().NotEmpty(loginEnvelope.Data.Tokens.AccessExpiresAt)
	s.Require().NotEmpty(loginEnvelope.Data.Tokens.RefreshToken)
	s.Require().NotEmpty(loginEnvelope.Data.Tokens.RefreshExpiresAt)

	patchBody, err := json.Marshal(map[string]any{
		"first_name": "Qobul",
		"last_name":  "Qobulov",
		"language":   "ru",
		"avatar_url": "https://example.com/avatar.jpg",
		"is_active":  false,
		"created_at": "2000-01-01T00:00:00Z",
	})
	s.Require().NoError(err)
	patchRequest := httptest.NewRequest("PATCH", "/api/v1/me", bytes.NewBuffer(patchBody))
	patchRequest.Header.Set("Content-Type", "application/json")
	// Swagger UI sends apiKey values exactly as entered, without adding a
	// Bearer prefix. Raw access tokens must therefore work on protected routes.
	patchRequest.Header.Set("Authorization", loginEnvelope.Data.Tokens.AccessToken)
	patchResponse, err := s.app.Test(patchRequest, -1)
	s.Require().NoError(err)
	defer patchResponse.Body.Close()
	s.Require().Equal(fiber.StatusOK, patchResponse.StatusCode)

	var patchEnvelope struct {
		Data struct {
			FirstName string `json:"first_name"`
			LastName  string `json:"last_name"`
			Language  string `json:"language"`
			IsActive  bool   `json:"is_active"`
		} `json:"data"`
	}
	s.Require().NoError(json.NewDecoder(patchResponse.Body).Decode(&patchEnvelope))
	s.Equal("Qobul", patchEnvelope.Data.FirstName)
	s.Equal("Qobulov", patchEnvelope.Data.LastName)
	s.Equal("ru", patchEnvelope.Data.Language)
	s.True(patchEnvelope.Data.IsActive, "database-managed is_active must be ignored")
}

func (s *PublicRoutesTestSuite) createLoginUser(username, phone, password string) {
	s.T().Helper()
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	s.Require().NoError(err)

	_, err = s.db.Exec(s.T().Context(), `
		INSERT INTO users (
			id, password_hash, name, phone, username, is_active, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, true, now(), now())
	`, uuid.New(), string(passwordHash), username, phone, username)
	s.Require().NoError(err)
}

// === ORDER ROUTES ===

func (s *PublicRoutesTestSuite) TestGetOrders() {
	req := httptest.NewRequest("GET", "/api/v1/orders", nil)
	resp, err := s.app.Test(req, -1)
	s.NoError(err)
	s.Equal(fiber.StatusOK, resp.StatusCode)
}

func (s *PublicRoutesTestSuite) TestGetOrderByID_NotFound() {
	req := httptest.NewRequest("GET", "/api/v1/orders/999", nil)
	resp, err := s.app.Test(req, -1)
	s.NoError(err)
	s.NotEqual(fiber.StatusInternalServerError, resp.StatusCode)
}

func (s *PublicRoutesTestSuite) TestCreateOrder() {
	body := map[string]interface{}{
		"total": 300,
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/api/v1/orders", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.app.Test(req, -1)
	s.NoError(err)
	s.True(resp.StatusCode == fiber.StatusOK || resp.StatusCode == fiber.StatusCreated)
}

func (s *PublicRoutesTestSuite) TestPatchOrder() {
	// First create an order
	createBody := map[string]interface{}{
		"total": 300,
	}
	createJsonBody, _ := json.Marshal(createBody)
	createReq := httptest.NewRequest("POST", "/api/v1/orders", bytes.NewBuffer(createJsonBody))
	createReq.Header.Set("Content-Type", "application/json")
	createResp, _ := s.app.Test(createReq, -1)
	s.True(createResp.StatusCode == fiber.StatusOK || createResp.StatusCode == fiber.StatusCreated)

	// Then try to patch it
	body := map[string]interface{}{
		"total": 3001,
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest("PATCH", "/api/v1/orders/1", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.app.Test(req, -1)
	s.NoError(err)
	s.True(resp.StatusCode >= 200 && resp.StatusCode < 500)
}

func (s *PublicRoutesTestSuite) TestDeleteOrder() {
	// First create an order
	createBody := map[string]interface{}{
		"total": 300,
	}
	createJsonBody, _ := json.Marshal(createBody)
	createReq := httptest.NewRequest("POST", "/api/v1/orders", bytes.NewBuffer(createJsonBody))
	createReq.Header.Set("Content-Type", "application/json")
	createResp, _ := s.app.Test(createReq, -1)
	s.True(createResp.StatusCode == fiber.StatusOK || createResp.StatusCode == fiber.StatusCreated)

	// Then try to delete it
	req := httptest.NewRequest("DELETE", "/api/v1/orders/1", nil)
	resp, err := s.app.Test(req, -1)
	s.NoError(err)
	s.True(resp.StatusCode >= 200 && resp.StatusCode < 500)
}
