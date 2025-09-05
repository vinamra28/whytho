package server

import (
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/vinamra28/operator-reviewer/internal/config"
	"github.com/vinamra28/operator-reviewer/internal/handlers"
	"github.com/vinamra28/operator-reviewer/internal/services"
)

type Server struct {
	config *config.Config
	router *gin.Engine
}

func New(cfg *config.Config) *Server {
	logrus.Info("Initializing server")

	router := gin.Default()

	logrus.Info("Creating GitLab service")
	gitlabService := services.NewGitLabService(cfg.GitLabToken, cfg.GitLabBaseURL)

	logrus.Info("Creating review service")
	reviewService := services.NewReviewService(cfg.GeminiAPIKey)

	logrus.Info("Creating webhook handler")
	webhookHandler := handlers.NewWebhookHandler(gitlabService, reviewService, cfg.WebhookSecret)

	logrus.Info("Setting up routes")
	router.POST("/webhook", webhookHandler.HandleWebhook)
	router.GET("/health", handlers.HealthCheck)

	logrus.Info("Server initialized successfully")
	return &Server{
		config: cfg,
		router: router,
	}
}

func (s *Server) Start(addr string) error {
	logrus.WithField("address", addr).Info("Starting HTTP server")
	return s.router.Run(addr)
}
