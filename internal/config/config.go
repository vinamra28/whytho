package config

import (
	"fmt"
	"os"

	"github.com/sirupsen/logrus"
)

type Config struct {
	GitLabToken   string
	GitLabBaseURL string
	GeminiAPIKey  string
	WebhookSecret string
}

func Load() (*Config, error) {
	logrus.Debug("Loading configuration from environment variables")

	cfg := &Config{
		GitLabToken:   os.Getenv("GITLAB_TOKEN"),
		GitLabBaseURL: os.Getenv("GITLAB_BASE_URL"),
		GeminiAPIKey:  os.Getenv("GEMINI_API_KEY"),
		WebhookSecret: os.Getenv("WEBHOOK_SECRET"),
	}

	if cfg.GitLabToken == "" {
		logrus.Error("GITLAB_TOKEN environment variable is missing")
		return nil, fmt.Errorf("GITLAB_TOKEN environment variable is required")
	}

	if cfg.GeminiAPIKey == "" {
		logrus.Error("GEMINI_API_KEY environment variable is missing")
		return nil, fmt.Errorf("GEMINI_API_KEY environment variable is required")
	}

	if cfg.GitLabBaseURL == "" {
		cfg.GitLabBaseURL = "https://gitlab.com"
		logrus.WithField("url", cfg.GitLabBaseURL).Info("Using default GitLab base URL")
	} else {
		logrus.WithField("url", cfg.GitLabBaseURL).Info("Using custom GitLab base URL")
	}

	if cfg.WebhookSecret == "" {
		logrus.Warn("WEBHOOK_SECRET not set - webhook signature verification disabled")
	} else {
		logrus.Info("Webhook signature verification enabled")
	}

	logrus.Debug("Configuration loaded successfully")
	return cfg, nil
}
