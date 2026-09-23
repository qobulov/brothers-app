package app

import (
	"context"
	"log"
	"os"
	"strings"

	"github.com/qobulov/brothers-app/internal/auth/otp"
	authService "github.com/qobulov/brothers-app/internal/auth/service"
	"github.com/qobulov/brothers-app/internal/auth/telegram"
	cachepkg "github.com/qobulov/brothers-app/pkg/cache"
	"github.com/qobulov/brothers-app/pkg/config"
	"github.com/qobulov/brothers-app/pkg/database"
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
	restApp, err := SetupRestServer(pool, otpCache, cfg)
	if err != nil {
		log.Fatalf("❌ Failed to setup REST server: %v", err)
	}

	workerContext, stopWorker := context.WithCancel(context.Background())
	if shouldStartTelegramPolling(cfg) {
		telegramClient := telegram.NewClient(cfg.TelegramBotToken, cfg.TelegramBotAPIURL, cfg.TelegramHTTPTimeout, cfg.TelegramPollTimeout)
		auth := authService.New(pool, otpCache, cfg, telegramClient)
		worker := telegram.NewWorker(telegramClient, auth)
		go func() {
			if err := worker.Run(workerContext); err != nil && err != context.Canceled {
				log.Printf("telegram worker stopped: %v", err)
			}
		}()
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
			stopWorker()
		},
		func() {
			if err := database.Close(); err != nil {
				log.Printf("Error closing DB: %v", err)
			}
		},
		func() {
			if err := redisClient.Close(); err != nil {
				log.Printf("Error closing Redis: %v", err)
			}
		},
	})

}

func shouldStartTelegramPolling(cfg *config.Config) bool {
	if cfg == nil || strings.TrimSpace(cfg.TelegramBotToken) == "" {
		return false
	}
	// Telegram webhook and getUpdates are mutually exclusive. Vercel receives
	// updates through the webhook route; persistent local/server processes poll.
	return strings.TrimSpace(cfg.TelegramWebhookSecret) == "" && os.Getenv("VERCEL") == ""
}
