package app

import (
	"context"
	"log"

	"github.com/qobulov/brothers-app/internal/auth/otp"
	"github.com/qobulov/brothers-app/internal/auth/session"
	cachepkg "github.com/qobulov/brothers-app/pkg/cache"
	"github.com/qobulov/brothers-app/utils"
)

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
	restApp, err := SetupRestServer(pool, otpCache, session.NewRedisStore(redisClient), cfg)
	if err != nil {
		log.Fatalf("❌ Failed to setup REST server: %v", err)
	}

	// Start REST server
	go utils.StartRestServer(restApp, cfg)

	// Graceful shutdown listener
	utils.WaitForShutdown([]func(){
		func() {
			log.Println("Shutting down REST server...")
			if err := restApp.Shutdown(); err != nil {
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
