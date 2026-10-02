package routes_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/suite"
	"golang.org/x/crypto/bcrypt"

	"github.com/qobulov/brothers-app/internal/app"
	"github.com/qobulov/brothers-app/internal/auth/session"
	"github.com/qobulov/brothers-app/pkg/config"
	"github.com/qobulov/brothers-app/pkg/database"
)

type PublicRoutesTestSuite struct {
	suite.Suite
	db          *pgxpool.Pool
	app         *fiber.App
	cfg         *config.Config
	accessToken string
	cleanup     func()
}

func (s *PublicRoutesTestSuite) SetupTest() {
	// Setup test database with cleanup
	s.db, s.cleanup = database.SetupTestDB(s.T())

	// Load config for dev environment
	s.cfg = config.LoadConfig("dev")
	// Tests must never post failures to the real Telegram error topic.
	s.cfg.TelegramBotToken = ""

	// Setup REST server with test database (For registering routes and middleware)
	var err error
	s.app, err = app.SetupRestServer(s.db, nil, session.NewMemoryStore(), s.cfg)
	s.NoError(err, "Failed to setup REST server")
	s.createLoginUser("suite-owner", "+998901239999", "suite-owner@example.com", "securepassword123")
	s.accessToken = s.loginAccessToken("suite-owner", "securepassword123")
}

func (s *PublicRoutesTestSuite) TearDownTest() {
	if s.app != nil {
		s.Require().NoError(s.app.Shutdown())
	}
	// Clean up database after each test
	if s.cleanup != nil {
		s.cleanup()
	}
}

func TestPublicRoutesTestSuite(t *testing.T) {
	suite.Run(t, new(PublicRoutesTestSuite))
}

// === AUTH ROUTES ===

func (s *PublicRoutesTestSuite) TestLogin() {
	const password = "securepassword123"
	s.createLoginUser("loginuser", "+998901234567", "loginuser@example.com", password)

	tests := []struct {
		name  string
		login string
	}{
		{name: "username", login: "loginuser"},
		{name: "email", login: "loginuser@example.com"},
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

func (s *PublicRoutesTestSuite) TestLoginWithoutDeviceHeaders() {
	const password = "securepassword123"
	s.createLoginUser("device-user", "+998901234581", "device@example.com", password)

	body, err := json.Marshal(map[string]string{"login": "device-user", "password": password})
	s.Require().NoError(err)
	req := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.app.Test(req, -1)
	s.Require().NoError(err)
	defer resp.Body.Close()
	s.Require().Equal(fiber.StatusOK, resp.StatusCode)

}

func (s *PublicRoutesTestSuite) TestUserLookup() {
	const password = "securepassword123"
	s.createLoginUser("lookup-owner", "+998901234579", "lookup-owner@example.com", password)
	s.createLoginUser("lookup-target", "+998901234580", "lookup-target@example.com", password)

	loginBody, err := json.Marshal(map[string]string{"login": "lookup-owner", "password": password})
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
				AccessToken string `json:"access_token"`
			} `json:"tokens"`
		} `json:"data"`
	}
	s.Require().NoError(json.NewDecoder(loginResponse.Body).Decode(&loginEnvelope))

	lookupRequest := httptest.NewRequest("GET", "/api/v1/users?query=lookup-target", nil)
	lookupRequest.Header.Set("Authorization", loginEnvelope.Data.Tokens.AccessToken)
	lookupResponse, err := s.app.Test(lookupRequest, -1)
	s.Require().NoError(err)
	defer lookupResponse.Body.Close()
	s.Require().Equal(fiber.StatusOK, lookupResponse.StatusCode)

	var lookupEnvelope struct {
		Data []struct {
			ID        string `json:"id"`
			Username  string `json:"username"`
			Email     string `json:"email"`
			AvatarURL string `json:"avatar_url"`
		} `json:"data"`
	}
	s.Require().NoError(json.NewDecoder(lookupResponse.Body).Decode(&lookupEnvelope))
	s.Require().Len(lookupEnvelope.Data, 1)
	s.Equal("lookup-target", lookupEnvelope.Data[0].Username)
	s.Equal("lookup-target@example.com", lookupEnvelope.Data[0].Email)
}

func (s *PublicRoutesTestSuite) TestUserLookup_RequiresAuthentication() {
	req := httptest.NewRequest("GET", "/api/v1/users", nil)
	resp, err := s.app.Test(req, -1)
	s.Require().NoError(err)
	defer resp.Body.Close()
	s.Equal(fiber.StatusUnauthorized, resp.StatusCode)
}

func (s *PublicRoutesTestSuite) TestLegacyUserCRUDRoutesNotRegistered() {
	legacyRoutes := map[string]bool{
		"GET /api/v1/users/:id":    false,
		"PATCH /api/v1/users/:id":  false,
		"DELETE /api/v1/users/:id": false,
		"GET /api/v1/users/me":     false,
	}
	for _, route := range s.app.GetRoutes() {
		key := route.Method + " " + route.Path
		if _, ok := legacyRoutes[key]; ok {
			legacyRoutes[key] = true
		}
	}
	for route, registered := range legacyRoutes {
		if registered {
			s.T().Errorf("legacy user route %s is still registered", route)
		}
	}
}

func (s *PublicRoutesTestSuite) TestLegacyAuthRoutesNotRegistered() {
	legacyPaths := map[string]bool{
		"/api/v1/auth/signin":             false,
		"/api/v1/auth/signup":             false,
		"/api/v1/auth/otp/verify":         false,
		"/api/v1/auth/register/resend":    false,
		"/api/v1/auth/password/forgot":    false,
		"/api/v1/auth/password/resend":    false,
		"/api/v1/me/phone-change/resend":  false,
		"/api/v1/telegram/webhook":        false,
		"/api/v1/me/phone-change/request": false,
		"/api/v1/me/phone-change/confirm": false,
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

func (s *PublicRoutesTestSuite) TestGroupDetailGetRouteNotRegistered() {
	for _, route := range s.app.GetRoutes() {
		if route.Method == fiber.MethodGet && route.Path == "/api/v1/groups/:groupID" {
			s.T().Fatal("GET /api/v1/groups/:groupID must not be registered")
		}
	}
}

func (s *PublicRoutesTestSuite) TestRegisterConflictLocalized() {
	s.createLoginUser("existing-user", "+998901234577", "existing@example.com", "securepassword123")

	tests := []struct {
		name     string
		language string
		want     string
	}{
		{name: "uzbek", language: "uz", want: "Email, telefon raqami yoki foydalanuvchi nomi allaqachon mavjud"},
		{name: "russian", language: "ru", want: "Email, номер телефона или имя пользователя уже существуют"},
		{name: "english", language: "en", want: "Email, phone, or username already exists"},
	}

	for _, test := range tests {
		s.Run(test.name, func() {
			body, err := json.Marshal(map[string]string{
				"email":      "existing@example.com",
				"phone":      "+998901234577",
				"username":   "new_user_" + test.language,
				"first_name": "Azizbek",
				"last_name":  "Qobulov",
				"password":   "strong-password",
				"language":   test.language,
				"otp_code":   "111111",
			})
			s.Require().NoError(err)

			req := httptest.NewRequest("POST", "/api/v1/auth/register", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set(fiber.HeaderAcceptLanguage, test.language)
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
			s.Equal("email_phone_or_username_exists", envelope.Slug)
			s.Equal(test.want, envelope.Message)
		})
	}
}

func (s *PublicRoutesTestSuite) TestCurrentProfilePatch() {
	const password = "securepassword123"
	s.createLoginUser("profileuser", "+998901234568", "profileuser@example.com", password)

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
		"first_name": "Azizbek",
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
	s.Equal("Azizbek", patchEnvelope.Data.FirstName)
	s.Equal("Qobulov", patchEnvelope.Data.LastName)
	s.Equal("ru", patchEnvelope.Data.Language)
	s.True(patchEnvelope.Data.IsActive, "database-managed is_active must be ignored")
}

func (s *PublicRoutesTestSuite) TestDeleteGroupRequiresConfirmationAndLocalizesResponse() {
	const password = "securepassword123"
	s.createLoginUser("delete-group-owner", "+998901234569", "delete-group-owner@example.com", password)

	loginBody, err := json.Marshal(map[string]string{
		"login":    "delete-group-owner",
		"password": password,
	})
	s.Require().NoError(err)
	loginRequest := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewReader(loginBody))
	loginRequest.Header.Set("Content-Type", "application/json")
	loginResponse, err := s.app.Test(loginRequest, -1)
	s.Require().NoError(err)
	defer loginResponse.Body.Close()
	s.Require().Equal(fiber.StatusOK, loginResponse.StatusCode)

	var loginEnvelope struct {
		Data struct {
			Tokens struct {
				AccessToken string `json:"access_token"`
			} `json:"tokens"`
		} `json:"data"`
	}
	s.Require().NoError(json.NewDecoder(loginResponse.Body).Decode(&loginEnvelope))

	createBody, err := json.Marshal(map[string]string{"name": "Delete through API"})
	s.Require().NoError(err)
	createRequest := httptest.NewRequest("POST", "/api/v1/groups", bytes.NewReader(createBody))
	createRequest.Header.Set("Content-Type", "application/json")
	createRequest.Header.Set("Authorization", loginEnvelope.Data.Tokens.AccessToken)
	createResponse, err := s.app.Test(createRequest, -1)
	s.Require().NoError(err)
	defer createResponse.Body.Close()
	s.Require().Equal(fiber.StatusCreated, createResponse.StatusCode)

	var createEnvelope struct {
		Data struct {
			ID uuid.UUID `json:"id"`
		} `json:"data"`
	}
	s.Require().NoError(json.NewDecoder(createResponse.Body).Decode(&createEnvelope))
	s.Require().NotEqual(uuid.Nil, createEnvelope.Data.ID)

	unconfirmedBody, err := json.Marshal(map[string]bool{"confirm": false})
	s.Require().NoError(err)
	unconfirmedRequest := httptest.NewRequest(
		"DELETE",
		"/api/v1/groups/"+createEnvelope.Data.ID.String(),
		bytes.NewReader(unconfirmedBody),
	)
	unconfirmedRequest.Header.Set("Content-Type", "application/json")
	unconfirmedRequest.Header.Set("Authorization", loginEnvelope.Data.Tokens.AccessToken)
	unconfirmedResponse, err := s.app.Test(unconfirmedRequest, -1)
	s.Require().NoError(err)
	defer unconfirmedResponse.Body.Close()
	s.Require().Equal(fiber.StatusBadRequest, unconfirmedResponse.StatusCode)

	confirmedBody, err := json.Marshal(map[string]bool{"confirm": true})
	s.Require().NoError(err)
	confirmedRequest := httptest.NewRequest(
		"DELETE",
		"/api/v1/groups/"+createEnvelope.Data.ID.String(),
		bytes.NewReader(confirmedBody),
	)
	confirmedRequest.Header.Set("Content-Type", "application/json")
	confirmedRequest.Header.Set("Authorization", loginEnvelope.Data.Tokens.AccessToken)
	confirmedRequest.Header.Set(fiber.HeaderAcceptLanguage, "ru")
	confirmedResponse, err := s.app.Test(confirmedRequest, -1)
	s.Require().NoError(err)
	defer confirmedResponse.Body.Close()
	s.Require().Equal(fiber.StatusOK, confirmedResponse.StatusCode)

	var deleteEnvelope struct {
		Success bool   `json:"success"`
		Slug    string `json:"slug"`
		Message string `json:"message"`
	}
	s.Require().NoError(json.NewDecoder(confirmedResponse.Body).Decode(&deleteEnvelope))
	s.True(deleteEnvelope.Success)
	s.Equal("ok", deleteEnvelope.Slug)
	s.Equal("Группа удалена", deleteEnvelope.Message)
}

func (s *PublicRoutesTestSuite) createLoginUser(username, phone, email, password string) {
	s.T().Helper()
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	s.Require().NoError(err)

	_, err = s.db.Exec(s.T().Context(), `
		INSERT INTO users (
			id, password_hash, name, email, phone, username, is_active, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, true, now(), now())
	`, uuid.New(), string(passwordHash), username, email, phone, username)
	s.Require().NoError(err)
}

func (s *PublicRoutesTestSuite) loginAccessToken(login, password string) string {
	s.T().Helper()
	body, err := json.Marshal(map[string]string{"login": login, "password": password})
	s.Require().NoError(err)
	request := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response, err := s.app.Test(request, -1)
	s.Require().NoError(err)
	defer response.Body.Close()
	s.Require().Equal(fiber.StatusOK, response.StatusCode)
	var envelope struct {
		Data struct {
			Tokens struct {
				AccessToken string `json:"access_token"`
			} `json:"tokens"`
		} `json:"data"`
	}
	s.Require().NoError(json.NewDecoder(response.Body).Decode(&envelope))
	s.Require().NotEmpty(envelope.Data.Tokens.AccessToken)
	return envelope.Data.Tokens.AccessToken
}

// === ORDER ROUTES ===

func (s *PublicRoutesTestSuite) TestLegacyOrderRoutesNotRegistered() {
	legacyRoutes := map[string]bool{
		"GET /api/v1/orders":        false,
		"POST /api/v1/orders":       false,
		"GET /api/v1/orders/:id":    false,
		"PATCH /api/v1/orders/:id":  false,
		"DELETE /api/v1/orders/:id": false,
	}
	for _, route := range s.app.GetRoutes() {
		key := route.Method + " " + route.Path
		if _, ok := legacyRoutes[key]; ok {
			legacyRoutes[key] = true
		}
	}
	for route, registered := range legacyRoutes {
		if registered {
			s.T().Errorf("legacy order route %s is still registered", route)
		}
	}
}

func (s *PublicRoutesTestSuite) TestGroupOrderRoutesRequireAuthentication() {
	request := httptest.NewRequest("GET", "/api/v1/groups/"+uuid.NewString()+"/orders", nil)
	response, err := s.app.Test(request, -1)
	s.Require().NoError(err)
	defer response.Body.Close()
	s.Equal(fiber.StatusUnauthorized, response.StatusCode)
}

func (s *PublicRoutesTestSuite) TestGroupOrderFlow() {
	groupID := s.createGroup("Order Flow")
	giverID := s.addEmployee(groupID, "flow-giver")
	receiverID := s.addEmployee(groupID, "flow-receiver")

	status, body := s.sendJSON("POST", "/api/v1/groups/"+groupID+"/orders", map[string]any{
		"giver_user_id": giverID, "giver_customer_phone": "+998901111111",
		"receiver_user_id": receiverID, "receiver_customer_phone": "+998902222222",
		"amount_usd": 7000, "fee_uzs": 50000,
	})
	s.Require().Equal(fiber.StatusCreated, status, string(body))
	var created struct {
		Data struct {
			ID    uuid.UUID `json:"id"`
			State string    `json:"state"`
		} `json:"data"`
	}
	s.Require().NoError(json.Unmarshal(body, &created))
	s.Equal("waiting_for_confirmation", created.Data.State)
	orderPath := "/api/v1/groups/" + groupID + "/orders/" + created.Data.ID.String()

	status, body = s.sendJSON("GET", "/api/v1/groups/"+groupID+"/orders", nil)
	s.Require().Equal(fiber.StatusOK, status, string(body))
	var list struct {
		Data []json.RawMessage `json:"data"`
	}
	s.Require().NoError(json.Unmarshal(body, &list))
	s.Len(list.Data, 1)

	status, _ = s.sendJSON("GET", "/api/v1/groups/"+groupID+"/orders?limit=101", nil)
	s.Equal(fiber.StatusBadRequest, status)
	status, _ = s.sendJSON("GET", orderPath, nil)
	s.Equal(fiber.StatusOK, status)
	status, _ = s.sendJSON("GET", orderPath+"/events", nil)
	s.Equal(fiber.StatusOK, status)
	status, _ = s.sendJSON("PATCH", orderPath, map[string]any{"amount_usd": 6800})
	s.Equal(fiber.StatusOK, status)
	status, _ = s.sendJSON("POST", orderPath+"/confirmations", map[string]any{"amount_usd": 6800})
	s.Equal(fiber.StatusForbidden, status, "a manager is not a party and cannot confirm")
	status, body = s.sendJSON("POST", orderPath+"/cancellation", map[string]any{"reason": "created by mistake"})
	s.Require().Equal(fiber.StatusCreated, status, string(body))
	s.Contains(string(body), `"status":"cancelled"`, "the creator cancels an unconfirmed order immediately")
	status, _ = s.sendJSON("POST", orderPath+"/cancellation/action", map[string]any{"action": "approve"})
	s.Equal(fiber.StatusForbidden, status, "a manager is not a party and cannot answer a cancellation")
}

func (s *PublicRoutesTestSuite) TestGroupMemberFlow() {
	groupID := s.createGroup("Member Flow")
	employeeID := s.addEmployee(groupID, "flow-employee")
	memberPath := "/api/v1/groups/" + groupID + "/members/" + employeeID

	status, body := s.sendJSON("GET", memberPath, nil)
	s.Require().Equal(fiber.StatusOK, status, string(body))
	s.Contains(string(body), `"can_remove":true`)
	s.NotContains(string(body), `"phone"`, "users have no phone number")

	status, body = s.sendJSON("POST", memberPath+"/balance-adjustments", map[string]any{"new_balance_usd": 7500, "reason": "Cash correction"})
	s.Require().Equal(fiber.StatusCreated, status, string(body))
	s.Contains(string(body), `"direction":"increase"`)

	status, body = s.sendJSON("GET", memberPath+"/balance-adjustments?limit=10", nil)
	s.Require().Equal(fiber.StatusOK, status, string(body))
	s.Contains(string(body), `"current_balance_usd":7500`)
	status, _ = s.sendJSON("GET", memberPath+"/balance-adjustments?limit=abc", nil)
	s.Equal(fiber.StatusBadRequest, status)

	status, _ = s.sendJSON("DELETE", memberPath, nil)
	s.Equal(fiber.StatusConflict, status, "a member with a balance cannot be removed")
	status, _ = s.sendJSON("POST", memberPath+"/balance-adjustments", map[string]any{"new_balance_usd": 0})
	s.Require().Equal(fiber.StatusCreated, status)
	status, body = s.sendJSON("PATCH", memberPath, map[string]any{"role": "investor"})
	s.Require().Equal(fiber.StatusOK, status, string(body))
	s.Contains(string(body), `"role":"investor"`)
	status, _ = s.sendJSON("DELETE", memberPath, nil)
	s.Equal(fiber.StatusOK, status)
	status, _ = s.sendJSON("GET", memberPath, nil)
	s.Equal(fiber.StatusNotFound, status)
}

// Reproduces the mobile app's request: the specific reason must come back in
// message, in the language from Application-Language.
func (s *PublicRoutesTestSuite) TestErrorReasonIsLocalizedInMessage() {
	for language, want := range map[string]string{
		"uz": "Ro'yxatdan o'tishda faqat email yuboriladi, username yuborilmasligi kerak",
		"ru": "При регистрации отправляется только email, username указывать не нужно",
		"en": "Registration accepts email only; omit the username",
	} {
		body, err := json.Marshal(map[string]string{"email": "abror@example.com", "purpose": "registration", "username": "Abrorjon755"})
		s.Require().NoError(err)
		request := httptest.NewRequest("POST", "/api/v1/auth/otp/send", bytes.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Application-Language", language)
		response, err := s.app.Test(request, -1)
		s.Require().NoError(err)
		var envelope struct {
			Slug    string `json:"slug"`
			Message string `json:"message"`
		}
		s.Require().NoError(json.NewDecoder(response.Body).Decode(&envelope))
		response.Body.Close()
		s.Equal(fiber.StatusBadRequest, response.StatusCode, language)
		s.Equal("invalid_data", envelope.Slug, language)
		s.Equal(want, envelope.Message, language)
	}
}

// Every error response carries data.reason, so clients never have to handle null.
func (s *PublicRoutesTestSuite) TestErrorResponsesAlwaysHaveReason() {
	for _, tt := range []struct {
		name, method, path, token, wantSlug string
		wantStatus, wantCode                int
	}{
		{name: "missing token", method: "GET", path: "/api/v1/groups", wantStatus: 401, wantCode: 1401, wantSlug: "unauthorized"},
		{name: "invalid token", method: "GET", path: "/api/v1/groups", token: "Bearer not-a-jwt", wantStatus: 401, wantCode: 1401, wantSlug: "unauthorized"},
		{name: "unknown endpoint", method: "GET", path: "/api/v1/does-not-exist", token: s.accessToken, wantStatus: 404, wantCode: 1404, wantSlug: "not_found"},
	} {
		request := httptest.NewRequest(tt.method, tt.path, nil)
		if tt.token != "" {
			request.Header.Set("Authorization", tt.token)
		}
		response, err := s.app.Test(request, -1)
		s.Require().NoError(err)
		var envelope struct {
			Code int    `json:"code"`
			Slug string `json:"slug"`
			Data *struct {
				Reason string `json:"reason"`
			} `json:"data"`
		}
		s.Require().NoError(json.NewDecoder(response.Body).Decode(&envelope))
		response.Body.Close()
		s.Equal(tt.wantStatus, response.StatusCode, tt.name)
		s.Equal(tt.wantCode, envelope.Code, tt.name)
		s.Equal(tt.wantSlug, envelope.Slug, tt.name)
		s.Require().NotNil(envelope.Data, tt.name+": data must not be null")
		s.NotEmpty(envelope.Data.Reason, tt.name)
	}
}

func (s *PublicRoutesTestSuite) checkUsername(username, language string) (int, string, bool, string) {
	s.T().Helper()
	request := httptest.NewRequest("GET", "/api/v1/auth/username/check?username="+url.QueryEscape(username), nil)
	request.Header.Set("Application-Language", language)
	response, err := s.app.Test(request, -1)
	s.Require().NoError(err)
	defer response.Body.Close()
	var envelope struct {
		Message string `json:"message"`
		Data    struct {
			Username  string `json:"username"`
			Available bool   `json:"available"`
		} `json:"data"`
	}
	s.Require().NoError(json.NewDecoder(response.Body).Decode(&envelope))
	return response.StatusCode, envelope.Data.Username, envelope.Data.Available, envelope.Message
}

func (s *PublicRoutesTestSuite) TestUsernameCheckWithoutToken() {
	// "qobulov" is free here; "taken_user" is the registered one.
	s.createLoginUser("taken_user", "+998901230001", "taken@example.com", "securepassword123")

	status, username, available, message := s.checkUsername("qobulov", "uz")
	s.Equal(fiber.StatusOK, status)
	s.Equal("qobulov", username)
	s.True(available)
	s.Equal("Username bo'sh", message)

	status, username, available, message = s.checkUsername("  TAKEN_USER ", "uz")
	s.Equal(fiber.StatusOK, status, "a taken username is not an error")
	s.Equal("TAKEN_USER", username, "the response keeps the typed case, trimmed")
	s.False(available, "uniqueness ignores case")
	s.Equal("Bu username band", message)

	status, _, _, message = s.checkUsername("abc", "uz")
	s.Equal(fiber.StatusBadRequest, status)
	s.Equal("Username 5 dan 32 belgigacha bo'lishi kerak", message)
	status, _, _, message = s.checkUsername("ali-vali", "en")
	s.Equal(fiber.StatusBadRequest, status)
	s.Equal("A username can contain only Latin letters, digits and underscores", message)
}

func (s *PublicRoutesTestSuite) TestRegisterKeepsUsernameCase() {
	status, body := s.sendJSON("POST", "/api/v1/auth/register", map[string]any{
		"email": "mixedcase@example.com", "username": "Mixed_Case1", "first_name": "Mixed",
		"password": "securepassword123", "otp_code": "111111",
	})
	s.Require().Equal(fiber.StatusOK, status, string(body))

	var stored string
	s.Require().NoError(s.db.QueryRow(s.T().Context(), `SELECT username FROM users WHERE email = 'mixedcase@example.com'`).Scan(&stored))
	s.Equal("Mixed_Case1", stored, "the username is stored as typed")

	s.NotEmpty(s.loginAccessToken("MIXED_CASE1", "securepassword123"), "login matches any capitalization")

	status, _ = s.sendJSON("POST", "/api/v1/auth/register", map[string]any{
		"email": "other@example.com", "username": "mixed_CASE1", "first_name": "Other",
		"password": "securepassword123", "otp_code": "111111",
	})
	s.Equal(fiber.StatusConflict, status, "the same username in another case is taken")
}

func (s *PublicRoutesTestSuite) TestDebtFlow() {
	status, body := s.sendJSON("GET", "/api/v1/debts/summary", nil)
	s.Require().Equal(fiber.StatusOK, status, string(body), "summary is not read as a debt ID")

	status, body = s.sendJSON("POST", "/api/v1/debts", map[string]any{
		"direction": "they_owe_me", "person_name": "Akmal", "currency": "USD", "amount": 1500,
	})
	s.Require().Equal(fiber.StatusCreated, status, string(body))
	var created struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	s.Require().NoError(json.Unmarshal(body, &created))
	debtPath := "/api/v1/debts/" + created.Data.ID

	status, body = s.sendJSON("POST", debtPath+"/repayments", map[string]any{"amount": 600})
	s.Require().Equal(fiber.StatusCreated, status, string(body))
	s.Contains(string(body), `"remaining_amount":900`)

	status, body = s.sendJSON("GET", debtPath+"/repayments", nil)
	s.Require().Equal(fiber.StatusOK, status)
	s.Contains(string(body), `"amount":600`)

	status, body = s.sendJSON("GET", "/api/v1/debts/summary", nil)
	s.Require().Equal(fiber.StatusOK, status)
	s.Contains(string(body), `"usd":{"they_owe_me":900,"i_owe":0}`)

	status, _ = s.sendJSON("GET", "/api/v1/debts?status=bogus", nil)
	s.Equal(fiber.StatusBadRequest, status)
	status, _ = s.sendJSON("POST", debtPath+"/complete", nil)
	s.Equal(fiber.StatusOK, status)
	status, _ = s.sendJSON("POST", debtPath+"/complete", nil)
	s.Equal(fiber.StatusConflict, status)
	status, _ = s.sendJSON("DELETE", debtPath, nil)
	s.Equal(fiber.StatusOK, status)
	status, _ = s.sendJSON("GET", debtPath, nil)
	s.Equal(fiber.StatusNotFound, status)

	request := httptest.NewRequest("GET", "/api/v1/debts", nil)
	response, err := s.app.Test(request, -1)
	s.Require().NoError(err)
	response.Body.Close()
	s.Equal(fiber.StatusUnauthorized, response.StatusCode, "debts require a token")
}

func (s *PublicRoutesTestSuite) createGroup(name string) string {
	s.T().Helper()
	status, body := s.sendJSON("POST", "/api/v1/groups", map[string]any{"name": name})
	s.Require().Equal(fiber.StatusCreated, status, string(body))
	var envelope struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	s.Require().NoError(json.Unmarshal(body, &envelope))
	return envelope.Data.ID
}

func (s *PublicRoutesTestSuite) addEmployee(groupID, username string) string {
	s.T().Helper()
	userID := uuid.NewString()
	_, err := s.db.Exec(s.T().Context(), `
		INSERT INTO users (id, email, username, language, is_active) VALUES ($1, $2, $3, 'uz', true)
	`, userID, username+"@example.com", username)
	s.Require().NoError(err)
	_, err = s.db.Exec(s.T().Context(), `
		INSERT INTO group_members (group_id, user_id, username, role) VALUES ($1, $2, $3, 'employee')
	`, groupID, userID, username)
	s.Require().NoError(err)
	return userID
}

func (s *PublicRoutesTestSuite) sendJSON(method, path string, payload any) (int, []byte) {
	s.T().Helper()
	var reader *bytes.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		s.Require().NoError(err)
		reader = bytes.NewReader(encoded)
	} else {
		reader = bytes.NewReader(nil)
	}
	request := httptest.NewRequest(method, path, reader)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", s.accessToken)
	response, err := s.app.Test(request, -1)
	s.Require().NoError(err)
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	s.Require().NoError(err)
	return response.StatusCode, body
}
