package services

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/vinamra28/whytho/internal/models"
	"github.com/xanzy/go-gitlab"
	"gopkg.in/yaml.v3"
)

type GitLabService struct {
	client  *gitlab.Client
	token   string
	baseURL string
}

func NewGitLabService(token, baseURL string) *GitLabService {
	logrus.WithField("base_url", baseURL).Info("Creating GitLab client")
	git, err := gitlab.NewClient(token, gitlab.WithBaseURL(baseURL))
	if err != nil {
		logrus.WithError(err).WithField("base_url", baseURL).Fatal("Failed to create GitLab client")
		panic(fmt.Sprintf("Failed to create GitLab client: %v", err))
	}

	logrus.Info("GitLab client created successfully")
	return &GitLabService{
		client:  git,
		token:   token,
		baseURL: baseURL,
	}
}

func (g *GitLabService) GetMRChanges(projectID, mrIID int) ([]models.MRChange, error) {
	logrus.WithFields(logrus.Fields{
		"project_id": projectID,
		"mr_iid":     mrIID,
	}).Debug("Fetching merge request changes")

	diffs, _, err := g.client.MergeRequests.ListMergeRequestDiffs(projectID, mrIID, nil)
	if err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{
			"project_id": projectID,
			"mr_iid":     mrIID,
		}).Error("Failed to fetch merge request changes from GitLab API")
		return nil, fmt.Errorf("failed to get MR changes: %w", err)
	}

	var mrChanges []models.MRChange
	for _, diff := range diffs {
		mrChange := models.MRChange{
			OldPath:     diff.OldPath,
			NewPath:     diff.NewPath,
			AMode:       diff.AMode,
			BMode:       diff.BMode,
			NewFile:     diff.NewFile,
			RenamedFile: diff.RenamedFile,
			DeletedFile: diff.DeletedFile,
			Diff:        diff.Diff,
		}
		mrChanges = append(mrChanges, mrChange)
	}

	logrus.WithFields(logrus.Fields{
		"project_id":    projectID,
		"mr_iid":        mrIID,
		"changes_count": len(mrChanges),
	}).Debug("Successfully fetched merge request changes")

	return mrChanges, nil
}

func (g *GitLabService) PostMRComment(projectID, mrIID int, comment string) error {
	logrus.WithFields(logrus.Fields{
		"project_id": projectID,
		"mr_iid":     mrIID,
	}).Debug("Posting comment to merge request")

	note := &gitlab.CreateMergeRequestNoteOptions{
		Body: &comment,
	}

	_, _, err := g.client.Notes.CreateMergeRequestNote(projectID, mrIID, note)
	if err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{
			"project_id": projectID,
			"mr_iid":     mrIID,
		}).Error("Failed to post comment to GitLab")
		return fmt.Errorf("failed to post MR comment: %w", err)
	}

	logrus.WithFields(logrus.Fields{
		"project_id": projectID,
		"mr_iid":     mrIID,
	}).Debug("Comment posted successfully to merge request")

	return nil
}

func (g *GitLabService) PostPositionedMRComment(projectID, mrIID int, positionedComment models.PositionedComment) error {
	logrus.WithFields(logrus.Fields{
		"project_id":  projectID,
		"mr_iid":      mrIID,
		"file_path":   positionedComment.FilePath,
		"line_number": positionedComment.LineNumber,
		"line_type":   positionedComment.LineType,
	}).Debug("Posting positioned comment to merge request")

	// Get merge request details to get SHA values
	mr, err := g.GetMRDetails(projectID, mrIID)
	if err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{
			"project_id": projectID,
			"mr_iid":     mrIID,
		}).Error("Failed to get MR details for positioned comment")
		return fmt.Errorf("failed to get MR details: %w", err)
	}

	// Convert diff line number to actual line number if needed
	actualLineNumber, err := g.convertDiffLineToActualLine(projectID, mrIID, positionedComment)
	if err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{
			"project_id":  projectID,
			"mr_iid":      mrIID,
			"file_path":   positionedComment.FilePath,
			"line_number": positionedComment.LineNumber,
		}).Warn("Failed to convert diff line to actual line, falling back to general comment")

		severityFormatted := formatSeverity(positionedComment.Severity)
		return g.PostMRComment(projectID, mrIID, fmt.Sprintf("**File: %s (Line %d)** - %s\n\n%s",
			positionedComment.FilePath, positionedComment.LineNumber, severityFormatted, positionedComment.Comment))
	}

	// Create updated positioned comment with actual line number
	actualComment := positionedComment
	actualComment.LineNumber = actualLineNumber

	// Use the discussions API with proper SHA values
	err = g.postPositionedCommentHTTP(projectID, mrIID, actualComment,
		mr.DiffRefs.BaseSha, mr.DiffRefs.HeadSha, mr.DiffRefs.StartSha)
	if err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{
			"project_id":  projectID,
			"mr_iid":      mrIID,
			"file_path":   positionedComment.FilePath,
			"line_number": actualLineNumber,
		}).Error("Failed to post positioned comment to GitLab")

		// Fall back to posting a general comment
		logrus.WithFields(logrus.Fields{
			"project_id": projectID,
			"mr_iid":     mrIID,
			"file_path":  positionedComment.FilePath,
		}).Info("Falling back to general comment")

		severityFormatted := formatSeverity(positionedComment.Severity)
		return g.PostMRComment(projectID, mrIID, fmt.Sprintf("**File: %s (Line %d)** - %s\n\n%s",
			positionedComment.FilePath, positionedComment.LineNumber, severityFormatted, positionedComment.Comment))
	}

	logrus.WithFields(logrus.Fields{
		"project_id":         projectID,
		"mr_iid":             mrIID,
		"file_path":          positionedComment.FilePath,
		"diff_line_number":   positionedComment.LineNumber,
		"actual_line_number": actualLineNumber,
	}).Debug("Positioned comment posted successfully to merge request")

	return nil
}

func (g *GitLabService) postPositionedCommentHTTP(projectID, mrIID int, positionedComment models.PositionedComment, baseSHA, headSHA, startSHA string) error {
	// Construct the API URL for discussions
	url := fmt.Sprintf("%s/api/v4/projects/%d/merge_requests/%d/discussions",
		strings.TrimSuffix(g.baseURL, "/"), projectID, mrIID)

	// Create form data
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Add body with severity and color formatting
	severityFormatted := formatSeverity(positionedComment.Severity)
	commentBody := fmt.Sprintf("%s\n\n%s", severityFormatted, positionedComment.Comment)
	_ = writer.WriteField("body", commentBody)

	// Add position fields
	_ = writer.WriteField("position[position_type]", "text")
	_ = writer.WriteField("position[base_sha]", baseSHA)
	_ = writer.WriteField("position[head_sha]", headSHA)
	_ = writer.WriteField("position[start_sha]", startSHA)
	_ = writer.WriteField("position[new_path]", positionedComment.FilePath)
	_ = writer.WriteField("position[old_path]", positionedComment.FilePath)

	// Add line number based on line type
	switch positionedComment.LineType {
	case "new":
		_ = writer.WriteField("position[new_line]", strconv.Itoa(positionedComment.LineNumber))
	case "old":
		_ = writer.WriteField("position[old_line]", strconv.Itoa(positionedComment.LineNumber))
	default:
		// For context lines, include both line numbers
		_ = writer.WriteField("position[new_line]", strconv.Itoa(positionedComment.LineNumber))
		_ = writer.WriteField("position[old_line]", strconv.Itoa(positionedComment.LineNumber))
	}

	_ = writer.Close()

	// Create HTTP request
	req, err := http.NewRequest("POST", url, &buf)
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("PRIVATE-TOKEN", g.token)

	// Make the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make HTTP request: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GitLab API returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

func (g *GitLabService) convertDiffLineToActualLine(projectID, mrIID int, positionedComment models.PositionedComment) (int, error) {
	logrus.WithFields(logrus.Fields{
		"project_id":       projectID,
		"mr_iid":           mrIID,
		"file_path":        positionedComment.FilePath,
		"diff_line_number": positionedComment.LineNumber,
		"line_type":        positionedComment.LineType,
	}).Debug("Converting diff line number to actual line number")

	// Get merge request changes
	diffs, _, err := g.client.MergeRequests.ListMergeRequestDiffs(projectID, mrIID, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to get MR changes: %w", err)
	}

	// Find the specific file
	for _, diff := range diffs {
		if diff.NewPath == positionedComment.FilePath || diff.OldPath == positionedComment.FilePath {
			return g.findActualLineNumber(diff.Diff, positionedComment)
		}
	}

	return 0, fmt.Errorf("file %s not found in merge request changes", positionedComment.FilePath)
}

func (g *GitLabService) findActualLineNumber(diff string, positionedComment models.PositionedComment) (int, error) {
	lines := strings.Split(diff, "\n")
	oldLineNum := 0
	newLineNum := 0
	diffLineNum := 0

	for _, line := range lines {
		if strings.HasPrefix(line, "@@") {
			// Parse hunk header
			parts := strings.Split(line, " ")
			if len(parts) >= 3 {
				oldPart := strings.TrimPrefix(parts[1], "-")
				newPart := strings.TrimPrefix(parts[2], "+")

				if oldComma := strings.Index(oldPart, ","); oldComma > 0 {
					oldPart = oldPart[:oldComma]
				}
				if newComma := strings.Index(newPart, ","); newComma > 0 {
					newPart = newPart[:newComma]
				}

				if oldStart, err := strconv.Atoi(oldPart); err == nil {
					oldLineNum = oldStart - 1
				}
				if newStart, err := strconv.Atoi(newPart); err == nil {
					newLineNum = newStart - 1
				}
			}
			continue
		}

		if strings.HasPrefix(line, "+") {
			newLineNum++
			diffLineNum++
			if positionedComment.LineType == "new" && diffLineNum == positionedComment.LineNumber {
				logrus.WithFields(logrus.Fields{
					"diff_line":   diffLineNum,
					"actual_line": newLineNum,
					"line_type":   "new",
				}).Debug("Found actual line number for new line")
				return newLineNum, nil
			}
		} else if strings.HasPrefix(line, "-") {
			oldLineNum++
			diffLineNum++
			if positionedComment.LineType == "old" && diffLineNum == positionedComment.LineNumber {
				logrus.WithFields(logrus.Fields{
					"diff_line":   diffLineNum,
					"actual_line": oldLineNum,
					"line_type":   "old",
				}).Debug("Found actual line number for old line")
				return oldLineNum, nil
			}
		} else if strings.HasPrefix(line, " ") {
			oldLineNum++
			newLineNum++
			diffLineNum++
			if positionedComment.LineType == "context" && diffLineNum == positionedComment.LineNumber {
				logrus.WithFields(logrus.Fields{
					"diff_line":   diffLineNum,
					"actual_line": newLineNum,
					"line_type":   "context",
				}).Debug("Found actual line number for context line")
				return newLineNum, nil
			}
		}
	}

	return 0, fmt.Errorf("could not find actual line number for diff line %d", positionedComment.LineNumber)
}

func (g *GitLabService) GetMRDetails(projectID, mrIID int) (*gitlab.MergeRequest, error) {
	logrus.WithFields(logrus.Fields{
		"project_id": projectID,
		"mr_iid":     mrIID,
	}).Debug("Fetching merge request details")

	mr, _, err := g.client.MergeRequests.GetMergeRequest(projectID, mrIID, nil)
	if err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{
			"project_id": projectID,
			"mr_iid":     mrIID,
		}).Error("Failed to fetch merge request details from GitLab API")
		return nil, fmt.Errorf("failed to get MR details: %w", err)
	}

	logrus.WithFields(logrus.Fields{
		"project_id": projectID,
		"mr_iid":     mrIID,
		"mr_title":   mr.Title,
	}).Debug("Successfully fetched merge request details")

	return mr, nil
}

func (g *GitLabService) GetReviewGuidance(projectID int, branch string) (string, error) {
	logrus.WithFields(logrus.Fields{
		"project_id": projectID,
		"branch":     branch,
	}).Debug("Fetching review guidance from .whytho directory")

	guidancePath := ".whytho/guidance.md"

	// Try to fetch .whytho/guidance.md from the repository
	file, _, err := g.client.RepositoryFiles.GetFile(projectID, guidancePath, &gitlab.GetFileOptions{
		Ref: &branch,
	})
	if err != nil {
		// Check if it's a 404 error (file not found)
		if strings.Contains(err.Error(), "404") {
			logrus.WithFields(logrus.Fields{
				"project_id":    projectID,
				"branch":        branch,
				"guidance_path": guidancePath,
			}).Debug("No .whytho/guidance.md file found in repository")
			return "", nil // Return empty string, not an error
		}

		logrus.WithError(err).WithFields(logrus.Fields{
			"project_id":    projectID,
			"branch":        branch,
			"guidance_path": guidancePath,
		}).Error("Failed to fetch .whytho/guidance.md from repository")
		return "", fmt.Errorf("failed to fetch .whytho/guidance.md: %w", err)
	}

	// Decode the file content (GitLab returns base64 encoded content)
	content := file.Content
	if file.Encoding == "base64" {
		decoded, err := base64.StdEncoding.DecodeString(content)
		if err != nil {
			logrus.WithError(err).WithFields(logrus.Fields{
				"project_id":    projectID,
				"branch":        branch,
				"guidance_path": guidancePath,
			}).Error("Failed to decode .whytho/guidance.md content")
			return "", fmt.Errorf("failed to decode .whytho/guidance.md: %w", err)
		}
		content = string(decoded)
	}

	logrus.WithFields(logrus.Fields{
		"project_id":      projectID,
		"branch":          branch,
		"guidance_path":   guidancePath,
		"guidance_length": len(content),
	}).Info("Successfully fetched review guidance from .whytho directory")

	return content, nil
}

func (g *GitLabService) GetWhyThoConfig(projectID, mrIID int, targetBranch string, changes []models.MRChange) (*models.WhyThoConfig, error) {
	logrus.WithFields(logrus.Fields{
		"project_id": projectID,
		"mr_iid":     mrIID,
		"branch":     targetBranch,
	}).Debug("Fetching WhyTho config")

	configPath := ".whytho/config.yaml"

	// First, check if .whytho/config.yaml is modified in the MR diff
	for _, change := range changes {
		if change.NewPath == configPath && !change.DeletedFile {
			logrus.WithFields(logrus.Fields{
				"project_id": projectID,
				"mr_iid":     mrIID,
			}).Info("WhyTho config found in MR diff, using modified version")

			// Parse the new version from the diff
			config, err := g.parseWhyThoConfigFromDiff(change.Diff)
			if err != nil {
				logrus.WithError(err).WithFields(logrus.Fields{
					"project_id": projectID,
					"mr_iid":     mrIID,
				}).Warn("Failed to parse WhyTho config from diff, falling back to target branch")
				break // Fall through to target branch lookup
			}
			return config, nil
		}
	}

	// If not in diff, fetch from target branch
	return g.getWhyThoConfigFromBranch(projectID, targetBranch)
}

func (g *GitLabService) parseWhyThoConfigFromDiff(diff string) (*models.WhyThoConfig, error) {
	lines := strings.Split(diff, "\n")
	var yamlContent strings.Builder

	for _, line := range lines {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			// Remove the '+' prefix and add to YAML content
			yamlContent.WriteString(strings.TrimPrefix(line, "+") + "\n")
		}
	}

	var config models.WhyThoConfig
	if err := yaml.Unmarshal([]byte(yamlContent.String()), &config); err != nil {
		return nil, fmt.Errorf("failed to parse YAML from diff: %w", err)
	}

	return &config, nil
}

func (g *GitLabService) getWhyThoConfigFromBranch(projectID int, branch string) (*models.WhyThoConfig, error) {
	logrus.WithFields(logrus.Fields{
		"project_id": projectID,
		"branch":     branch,
	}).Debug("Fetching WhyTho config from target branch")

	configPath := ".whytho/config.yaml"

	// Try to fetch .whytho/config.yaml from the repository
	file, _, err := g.client.RepositoryFiles.GetFile(projectID, configPath, &gitlab.GetFileOptions{
		Ref: &branch,
	})
	if err != nil {
		// Check if it's a 404 error (file not found)
		if strings.Contains(err.Error(), "404") {
			logrus.WithFields(logrus.Fields{
				"project_id": projectID,
				"branch":     branch,
			}).Debug("No .whytho/config.yaml file found in repository")
			return &models.WhyThoConfig{ExcludePaths: []string{}}, nil // Return empty config
		}

		logrus.WithError(err).WithFields(logrus.Fields{
			"project_id": projectID,
			"branch":     branch,
		}).Error("Failed to fetch .whytho/config.yaml from repository")
		return nil, fmt.Errorf("failed to fetch .whytho/config.yaml: %w", err)
	}

	// Decode the file content (GitLab returns base64 encoded content)
	content := file.Content
	if file.Encoding == "base64" {
		decoded, err := base64.StdEncoding.DecodeString(content)
		if err != nil {
			logrus.WithError(err).WithFields(logrus.Fields{
				"project_id": projectID,
				"branch":     branch,
			}).Error("Failed to decode .whytho/config.yaml content")
			return nil, fmt.Errorf("failed to decode .whytho/config.yaml: %w", err)
		}
		content = string(decoded)
	}

	// Parse YAML content
	var config models.WhyThoConfig
	if err := yaml.Unmarshal([]byte(content), &config); err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{
			"project_id": projectID,
			"branch":     branch,
		}).Error("Failed to parse .whytho/config.yaml content")
		return nil, fmt.Errorf("failed to parse .whytho/config.yaml: %w", err)
	}

	logrus.WithFields(logrus.Fields{
		"project_id":    projectID,
		"branch":        branch,
		"exclude_paths": len(config.ExcludePaths),
	}).Info("Successfully fetched WhyTho config from repository")

	return &config, nil
}

func formatSeverity(severity string) string {
	switch severity {
	case "CRITICAL":
		return "![Critical](https://camo.githubusercontent.com/97cdeeadb5113837c0d106b7e5adaab2e2b698c5115c07d716586ff9f7c771eb/68747470733a2f2f7777772e677374617469632e636f6d2f636f64657265766965776167656e742f637269746963616c2e737667)"
	case "HIGH":
		return "![High](https://camo.githubusercontent.com/6be8dfd86afd9805fdf193065c11418ba1225c487bda10d961f6a486f586fa07/68747470733a2f2f7777772e677374617469632e636f6d2f636f64657265766965776167656e742f686967682d7072696f726974792e737667)"
	case "MEDIUM":
		return "![Medium](https://camo.githubusercontent.com/a33c7ba47779a2aabda978d90fc2665dd949d88da87f5ff554e65308de059f02/68747470733a2f2f7777772e677374617469632e636f6d2f636f64657265766965776167656e742f6d656469756d2d7072696f726974792e737667)"
	case "LOW":
		return "![Low](https://camo.githubusercontent.com/8fb8f2550573602557367af68a00a632c2f9e5ffbf5ade2572b4e0bfb7d3d33a/68747470733a2f2f7777772e677374617469632e636f6d2f636f64657265766965776167656e742f6c6f772d7072696f726974792e737667)"
	default:
		return fmt.Sprintf("**%s**", severity)
	}
}
