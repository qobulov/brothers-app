package app

import (
	"context"
	"log"

	authService "github.com/qobulov/brothers-app/internal/auth/service"
	"github.com/qobulov/brothers-app/internal/auth/telegram"
	"github.com/qobulov/brothers-app/pkg/database"
	"github.com/qobulov/brothers-app/utils"
)

func Start() {

	// Setup dependencies: database and configuration
	db, cfg, err := SetupDependencies("dev")
	if err != nil {
		log.Fatalf("❌ Failed to setup dependencies: %v", err)
	}

	// Setup REST server
	restApp, err := SetupRestServer(db, cfg)
	if err != nil {
		log.Fatalf("❌ Failed to setup REST server: %v", err)
	}

	workerContext, stopWorker := context.WithCancel(context.Background())
	if cfg.TelegramBotToken != "" {
		telegramClient := telegram.NewClient(cfg.TelegramBotToken, cfg.TelegramBotAPIURL, cfg.TelegramHTTPTimeout, cfg.TelegramPollTimeout)
		auth := authService.New(db, cfg, telegramClient)
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
	})

}
