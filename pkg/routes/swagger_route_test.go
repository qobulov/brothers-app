package routes

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestSwaggerRouteServesEmbeddedSpec(t *testing.T) {
	app := fiber.New()
	SwaggerRoute(app)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/swagger.json", nil)
	response, err := app.Test(request)
	if err != nil {
		t.Fatalf("request Swagger spec: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		t.Fatalf("Swagger spec status = %d, want %d", response.StatusCode, http.StatusOK)
	}
	if contentType := response.Header.Get("Content-Type"); contentType != "application/json" {
		t.Fatalf("Swagger spec Content-Type = %q, want %q", contentType, "application/json")
	}
}

func TestSwaggerRouteRedirectsRootToDocs(t *testing.T) {
	app := fiber.New()
	SwaggerRoute(app)

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response, err := app.Test(request)
	if err != nil {
		t.Fatalf("request root: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusTemporaryRedirect {
		t.Fatalf("root status = %d, want %d", response.StatusCode, http.StatusTemporaryRedirect)
	}
	if location := response.Header.Get("Location"); location != "/api/v1/docs" {
		t.Fatalf("root Location = %q, want %q", location, "/api/v1/docs")
	}
}

func TestSwaggerLoginUsesLoginRequestSchema(t *testing.T) {
	app := fiber.New()
	SwaggerRoute(app)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/swagger.json", nil)
	response, err := app.Test(request)
	if err != nil {
		t.Fatalf("request Swagger spec: %v", err)
	}
	defer response.Body.Close()

	var spec struct {
		Paths map[string]struct {
			Post struct {
				Parameters []struct {
					In     string `json:"in"`
					Schema struct {
						Ref string `json:"$ref"`
					} `json:"schema"`
				} `json:"parameters"`
			} `json:"post"`
		} `json:"paths"`
		Definitions map[string]struct {
			Properties map[string]json.RawMessage `json:"properties"`
		} `json:"definitions"`
	}
	if err := json.NewDecoder(response.Body).Decode(&spec); err != nil {
		t.Fatalf("decode Swagger spec: %v", err)
	}

	operation, ok := spec.Paths["/auth/login"]
	if !ok {
		t.Fatalf("login request body schema is missing")
	}
	var bodyRef string
	for _, parameter := range operation.Post.Parameters {
		if parameter.In == "body" {
			bodyRef = parameter.Schema.Ref
			break
		}
	}
	if bodyRef == "" {
		t.Fatal("login request body schema is missing")
	}
	definitionName := strings.TrimPrefix(bodyRef, "#/definitions/")
	definition, ok := spec.Definitions[definitionName]
	if !ok {
		t.Fatalf("login definition %q is missing", definitionName)
	}
	for _, field := range []string{"login", "password"} {
		if _, ok := definition.Properties[field]; !ok {
			t.Errorf("login request property %q is missing", field)
		}
	}
}

func TestSwaggerWriteRequestsExcludeDatabaseManagedFields(t *testing.T) {
	type operation struct {
		Parameters []struct {
			In     string `json:"in"`
			Schema struct {
				Ref string `json:"$ref"`
			} `json:"schema"`
		} `json:"parameters"`
	}
	type pathItem struct {
		Post  operation `json:"post"`
		Patch operation `json:"patch"`
	}

	app := fiber.New()
	SwaggerRoute(app)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/swagger.json", nil)
	response, err := app.Test(request)
	if err != nil {
		t.Fatalf("request Swagger spec: %v", err)
	}
	defer response.Body.Close()

	var spec struct {
		Paths       map[string]pathItem `json:"paths"`
		Definitions map[string]struct {
			Properties map[string]json.RawMessage `json:"properties"`
		} `json:"definitions"`
	}
	if err := json.NewDecoder(response.Body).Decode(&spec); err != nil {
		t.Fatalf("decode Swagger spec: %v", err)
	}
	for _, legacyPath := range []string{
		"/auth/signin",
		"/auth/signup",
		"/auth/otp/verify",
		"/auth/register/resend",
		"/auth/password/resend",
		"/me/phone-change/resend",
		"/me/phone-change/request",
		"/me/phone-change/confirm",
		"/telegram/webhook",
	} {
		if _, ok := spec.Paths[legacyPath]; ok {
			t.Errorf("legacy Swagger path %q must not be documented", legacyPath)
		}
	}

	tests := []struct {
		name            string
		operation       operation
		wantFields      []string
		forbiddenFields []string
	}{
		{name: "auth register", operation: spec.Paths["/auth/register"].Post, wantFields: []string{"email", "phone", "username", "password", "otp_code"}},
		{name: "auth otp send", operation: spec.Paths["/auth/otp/send"].Post, wantFields: []string{"email", "purpose"}},
		{name: "password verify", operation: spec.Paths["/auth/password/verify"].Post, wantFields: []string{"email", "otp"}},
		{name: "password reset", operation: spec.Paths["/auth/password/reset"].Post, wantFields: []string{"reset_token", "password"}, forbiddenFields: []string{"confirm_password"}},
		{name: "current profile patch", operation: spec.Paths["/me"].Patch, wantFields: []string{"first_name", "last_name", "avatar_url", "language", "username"}},
		{name: "group order create", operation: spec.Paths["/groups/{groupID}/orders"].Post, wantFields: []string{"giver_user_id", "giver_customer_phone", "receiver_user_id", "receiver_customer_phone", "amount_usd", "fee_uzs"}},
		{name: "email change", operation: spec.Paths["/me/email"].Post, wantFields: []string{"new_email", "otp_code"}},
		{name: "debt create", operation: spec.Paths["/debts"].Post, wantFields: []string{"direction", "person_name", "person_phone", "currency", "amount"}},
		{name: "debt repayment", operation: spec.Paths["/debts/{debtID}/repayments"].Post, wantFields: []string{"amount"}},
		{name: "member edit", operation: spec.Paths["/groups/{groupID}/members/{userID}"].Patch, wantFields: []string{"role", "location_id"}},
		{name: "member balance adjust", operation: spec.Paths["/groups/{groupID}/members/{userID}/balance-adjustments"].Post, wantFields: []string{"new_balance_usd", "reason"}},
		{name: "group order confirm", operation: spec.Paths["/groups/{groupID}/orders/{orderID}/confirmations"].Post, wantFields: []string{"amount_usd", "fee_uzs"}},
	}
	managedFields := []string{"id", "created_at", "updated_at", "deleted_at", "is_active", "last_login_at"}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var bodyRef string
			for _, parameter := range test.operation.Parameters {
				if parameter.In == "body" {
					bodyRef = parameter.Schema.Ref
					break
				}
			}
			if bodyRef == "" {
				t.Fatalf("request body parameter is missing")
			}
			definitionName := strings.TrimPrefix(bodyRef, "#/definitions/")
			definition, ok := spec.Definitions[definitionName]
			if !ok {
				t.Fatalf("request definition %q is missing", definitionName)
			}
			for _, field := range test.wantFields {
				if _, ok := definition.Properties[field]; !ok {
					t.Errorf("request property %q is missing", field)
				}
			}
			for _, field := range test.forbiddenFields {
				if _, ok := definition.Properties[field]; ok {
					t.Errorf("request property %q must not be accepted", field)
				}
			}
			for _, field := range managedFields {
				if _, ok := definition.Properties[field]; ok {
					t.Errorf("database-managed property %q must not be accepted", field)
				}
			}
		})
	}
}

func TestSwaggerDocumentsAuthContract(t *testing.T) {
	type operation struct {
		Security []map[string][]string `json:"security"`
	}
	type pathItem struct {
		Get   *operation `json:"get"`
		Post  *operation `json:"post"`
		Patch *operation `json:"patch"`
	}

	app := fiber.New()
	SwaggerRoute(app)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/swagger.json", nil)
	response, err := app.Test(request)
	if err != nil {
		t.Fatalf("request Swagger spec: %v", err)
	}
	defer response.Body.Close()

	var spec struct {
		Paths map[string]pathItem `json:"paths"`
	}
	if err := json.NewDecoder(response.Body).Decode(&spec); err != nil {
		t.Fatalf("decode Swagger spec: %v", err)
	}

	tests := []struct {
		method    string
		path      string
		protected bool
	}{
		{method: http.MethodPost, path: "/auth/register"},
		{method: http.MethodPost, path: "/auth/otp/send"},
		{method: http.MethodPost, path: "/auth/login"},
		{method: http.MethodPost, path: "/auth/refresh"},
		{method: http.MethodPost, path: "/auth/logout", protected: true},
		{method: http.MethodPost, path: "/auth/password/verify"},
		{method: http.MethodPost, path: "/auth/password/reset"},
		{method: http.MethodGet, path: "/me", protected: true},
		{method: http.MethodPatch, path: "/me", protected: true},
	}
	if _, exists := spec.Paths["/auth/password/forgot"]; exists {
		t.Error("removed password forgot endpoint is still documented")
	}

	for _, test := range tests {
		t.Run(test.method+" "+test.path, func(t *testing.T) {
			path, ok := spec.Paths[test.path]
			if !ok {
				t.Fatalf("Swagger path is missing")
			}
			var operation *operation
			switch test.method {
			case http.MethodGet:
				operation = path.Get
			case http.MethodPost:
				operation = path.Post
			case http.MethodPatch:
				operation = path.Patch
			}
			if operation == nil {
				t.Fatalf("Swagger operation is missing")
			}
			if test.protected {
				if len(operation.Security) == 0 {
					t.Fatal("protected operation has no BearerAuth security")
				}
				if _, ok := operation.Security[0]["BearerAuth"]; !ok {
					t.Fatal("protected operation does not use BearerAuth")
				}
			}
		})
	}
}
