package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/vinamra28/whytho/internal/models"
	"github.com/vinamra28/whytho/internal/services"
)

type WebhookHandler struct {
	gitlabService *services.GitLabService
	reviewService *services.ReviewService
	webhookSecret string
}

func NewWebhookHandler(gitlabService *services.GitLabService, reviewService *services.ReviewService, webhookSecret string) *WebhookHandler {
	logrus.Info("Creating webhook handler")
	return &WebhookHandler{
		gitlabService: gitlabService,
		reviewService: reviewService,
		webhookSecret: webhookSecret,
	}
}

func (h *WebhookHandler) HandleWebhook(c *gin.Context) {
	logrus.Info("Received webhook request")
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		logrus.WithError(err).Error("Failed to read request body")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read request body"})
		return
	}

	if h.webhookSecret != "" {
		logrus.Debug("Verifying webhook signature")
		if !h.verifySignature(body, c.GetHeader("X-Gitlab-Token")) {
			logrus.Warn("Invalid webhook signature received")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid signature"})
			return
		}
		logrus.Debug("Webhook signature verified successfully")
	}

	eventType := c.GetHeader("X-Gitlab-Event")
	logrus.WithField("event_type", eventType).Debug("Received GitLab event")
	if eventType != "Merge Request Hook" {
		logrus.WithField("event_type", eventType).Info("Ignoring non-merge request event")
		c.JSON(http.StatusOK, gin.H{"message": "Event ignored"})
		return
	}

	var webhook models.GitLabWebhook
	if err := json.Unmarshal(body, &webhook); err != nil {
		logrus.WithError(err).Error("Failed to parse webhook payload")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to parse webhook"})
		return
	}
	logrus.WithFields(logrus.Fields{
		"project_id": webhook.Project.ID,
		"mr_iid":     webhook.ObjectAttributes.IID,
		"action":     webhook.ObjectAttributes.Action,
	}).Info("Parsed webhook payload")

	if webhook.ObjectAttributes.Action != "open" && webhook.ObjectAttributes.Action != "reopen" && webhook.ObjectAttributes.Action != "update" {
		logrus.WithField("action", webhook.ObjectAttributes.Action).Info("Ignoring merge request action")
		c.JSON(http.StatusOK, gin.H{"message": "Action ignored"})
		return
	}

	// Check if this is a new commit (only for update actions)
	// The oldrev field is only present when commits are pushed to the MR
	if webhook.ObjectAttributes.Action == "update" && webhook.ObjectAttributes.OldRev == "" {
		logrus.WithFields(logrus.Fields{
			"project_id": webhook.Project.ID,
			"mr_iid":     webhook.ObjectAttributes.IID,
		}).Info("MR update without new commits (e.g., label/assignee change), skipping review")
		c.JSON(http.StatusOK, gin.H{"message": "No new commits, review skipped"})
		return
	}

	logrus.WithFields(logrus.Fields{
		"project_id": webhook.Project.ID,
		"mr_iid":     webhook.ObjectAttributes.IID,
	}).Info("Starting merge request processing")
	go h.processMergeRequest(&webhook)

	c.JSON(http.StatusOK, gin.H{"message": "Webhook received"})
}

func (h *WebhookHandler) verifySignature(body []byte, signature string) bool {
	if h.webhookSecret == "" {
		return true
	}

	mac := hmac.New(sha256.New, []byte(h.webhookSecret))
	mac.Write(body)
	expectedSignature := hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(signature), []byte(expectedSignature))
}

func (h *WebhookHandler) processMergeRequest(webhook *models.GitLabWebhook) {
	projectID := webhook.Project.ID
	mrIID := webhook.ObjectAttributes.IID

	logrus.WithFields(logrus.Fields{
		"project_id": projectID,
		"mr_iid":     mrIID,
	}).Info("Processing merge request")

	changes, err := h.gitlabService.GetMRChanges(projectID, mrIID)
	if err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{
			"project_id": projectID,
			"mr_iid":     mrIID,
		}).Error("Failed to fetch MR changes")
		return
	}

	if len(changes) == 0 {
		logrus.WithFields(logrus.Fields{
			"project_id": projectID,
			"mr_iid":     mrIID,
		}).Warn("No changes found in merge request")
		return
	}

	logrus.WithFields(logrus.Fields{
		"project_id":    projectID,
		"mr_iid":        mrIID,
		"changes_count": len(changes),
	}).Info("Retrieved merge request changes")

	logrus.WithFields(logrus.Fields{
		"project_id": projectID,
		"mr_iid":     mrIID,
	}).Info("Starting code review")

	review, err := h.reviewService.ReviewCode(changes, webhook.ObjectAttributes.Title, webhook.ObjectAttributes.Description, h.gitlabService, projectID, mrIID, webhook.ObjectAttributes.TargetBranch)
	if err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{
			"project_id": projectID,
			"mr_iid":     mrIID,
		}).Error("Failed to review code")
		return
	}

	logrus.WithFields(logrus.Fields{
		"project_id":                projectID,
		"mr_iid":                    mrIID,
		"general_comments_count":    len(review.Comments),
		"positioned_comments_count": len(review.PositionedComments),
	}).Info("Code review completed")

	// Post positioned comments first
	for i, posComment := range review.PositionedComments {
		logrus.WithFields(logrus.Fields{
			"project_id":                projectID,
			"mr_iid":                    mrIID,
			"comment_index":             i + 1,
			"total_positioned_comments": len(review.PositionedComments),
			"file_path":                 posComment.FilePath,
			"line_number":               posComment.LineNumber,
		}).Debug("Posting positioned review comment")

		if err := h.gitlabService.PostPositionedMRComment(projectID, mrIID, posComment); err != nil {
			logrus.WithError(err).WithFields(logrus.Fields{
				"project_id":    projectID,
				"mr_iid":        mrIID,
				"comment_index": i + 1,
				"file_path":     posComment.FilePath,
				"line_number":   posComment.LineNumber,
			}).Error("Failed to post positioned review comment")
		} else {
			logrus.WithFields(logrus.Fields{
				"project_id":    projectID,
				"mr_iid":        mrIID,
				"comment_index": i + 1,
				"file_path":     posComment.FilePath,
				"line_number":   posComment.LineNumber,
			}).Debug("Positioned review comment posted successfully")
		}
	}

	// Post general comments
	for i, comment := range review.Comments {
		logrus.WithFields(logrus.Fields{
			"project_id":             projectID,
			"mr_iid":                 mrIID,
			"comment_index":          i + 1,
			"total_general_comments": len(review.Comments),
		}).Debug("Posting general review comment")

		if err := h.gitlabService.PostMRComment(projectID, mrIID, comment); err != nil {
			logrus.WithError(err).WithFields(logrus.Fields{
				"project_id":    projectID,
				"mr_iid":        mrIID,
				"comment_index": i + 1,
			}).Error("Failed to post general review comment")
		} else {
			logrus.WithFields(logrus.Fields{
				"project_id":    projectID,
				"mr_iid":        mrIID,
				"comment_index": i + 1,
			}).Debug("General review comment posted successfully")
		}
	}

	if review.Summary != "" {
		logrus.WithFields(logrus.Fields{
			"project_id": projectID,
			"mr_iid":     mrIID,
		}).Info("Posting review summary")

		summaryComment := fmt.Sprintf("## 🤖 AI Code Review Summary\n\n%s", review.Summary)
		if err := h.gitlabService.PostMRComment(projectID, mrIID, summaryComment); err != nil {
			logrus.WithError(err).WithFields(logrus.Fields{
				"project_id": projectID,
				"mr_iid":     mrIID,
			}).Error("Failed to post summary comment")
		} else {
			logrus.WithFields(logrus.Fields{
				"project_id": projectID,
				"mr_iid":     mrIID,
			}).Info("Summary comment posted successfully")
		}
	}

	logrus.WithFields(logrus.Fields{
		"project_id": projectID,
		"mr_iid":     mrIID,
	}).Info("Merge request processing completed")
}

func HealthCheck(c *gin.Context) {
	logrus.Debug("Health check requested")
	c.JSON(http.StatusOK, gin.H{"status": "healthy"})
}
