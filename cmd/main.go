package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/vinamra28/whytho/internal/config"
	"github.com/vinamra28/whytho/internal/server"
)

func main() {
	logrus.SetFormatter(&logrus.JSONFormatter{})
	logrus.SetLevel(logrus.InfoLevel)

	logrus.Info("Starting whytho Bot")

	cfg, err := config.Load()
	if err != nil {
		logrus.WithError(err).Fatal("Failed to load configuration")
	}
	logrus.Info("Configuration loaded successfully")

	srv := server.New(cfg)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	logrus.WithField("port", port).Info("Starting server")

	go func() {
		if err := srv.Start(":" + port); err != nil && err != http.ErrServerClosed {
			logrus.WithError(err).Fatal("Server failed to start")
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logrus.Info("Received shutdown signal")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logrus.WithError(err).Error("Server forced to shutdown")
	} else {
		logrus.Info("Server shutdown gracefully")
	}
}
