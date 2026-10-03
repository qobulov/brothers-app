package app

import (
	"context"
	"log"
	"time"

	"github.com/qobulov/brothers-app/internal/auth/otp"
	"github.com/qobulov/brothers-app/internal/auth/session"
	cachepkg "github.com/qobulov/brothers-app/pkg/cache"
	"github.com/qobulov/brothers-app/utils"
)

// shutdownTimeout bounds how long in-flight requests may drain. Together with
// the 10s shutdown hook budget it stays inside a 30s orchestrator grace period.
const shutdownTimeout = 15 * time.Second

func Start() {

	// Setup dependencies: database and configuration
	pool, cfg, err := SetupDependencies("dev")
	if err != nil {
		log.Fatalf("❌ Failed to setup dependencies: %v", err)
	}

	// Setup REST server
	redisClient, err := cachepkg.ConnectRedis(context.Background(), cfg.RedisURL)
	if err != nil {
		log.Fatalf("❌ Failed to connect to Redis: %v", err)
	}
	otpCache := otp.NewCache(redisClient)
	restApp, err := SetupRestServer(pool, otpCache, session.NewRedisStore(redisClient), redisClient, cfg)
	if err != nil {
		log.Fatalf("❌ Failed to setup REST server: %v", err)
	}

	// Start REST server
	go utils.StartRestServer(restApp, cfg)

	// Graceful shutdown listener
	utils.WaitForShutdown([]func(){
		func() {
			log.Println("Shutting down REST server...")
			if err := restApp.ShutdownWithTimeout(shutdownTimeout); err != nil {
				log.Printf("Error shutting down REST server: %v", err)
			}
		},
		func() {
			log.Println("Closing database pool...")
			pool.Close()
		},
		func() {
			if err := redisClient.Close(); err != nil {
				log.Printf("Error closing Redis: %v", err)
			}
		},
	})

}
