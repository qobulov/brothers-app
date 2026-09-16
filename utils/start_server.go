package utils

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/qobulov/brothers-app/pkg/config"

	"github.com/gofiber/fiber/v2"
)

func StartRestServer(app *fiber.App, cfg *config.Config) {
	log.Println("Starting REST server on port:", cfg.AppPort)
	if err := app.Listen(":" + cfg.AppPort); err != nil {
		log.Fatalf("REST server error: %v", err)
	}
}

func WaitForShutdown(cleanups []func()) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)

	<-c // Wait for signal
	log.Println("Shutting down...")

	for _, cleanup := range cleanups {
		cleanup()
	}

	log.Println("Shutdown complete.")
}
