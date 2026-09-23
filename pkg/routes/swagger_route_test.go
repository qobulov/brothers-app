package routes

import (
	"net/http"
	"net/http/httptest"
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
