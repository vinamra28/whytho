package main

import (
	"os"

	"github.com/sirupsen/logrus"
	"github.com/vinamra28/operator-reviewer/internal/config"
	"github.com/vinamra28/operator-reviewer/internal/server"
)

func main() {
	logrus.SetFormatter(&logrus.JSONFormatter{})
	logrus.SetLevel(logrus.InfoLevel)

	logrus.Info("Starting GitLab MR Reviewer Bot")

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
	if err := srv.Start(":" + port); err != nil {
		logrus.WithError(err).Fatal("Server failed to start")
	}
}
