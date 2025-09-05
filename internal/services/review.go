package services

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/generative-ai-go/genai"
	"github.com/sirupsen/logrus"
	"github.com/vinamra28/operator-reviewer/internal/models"
	"google.golang.org/api/option"
)

type ReviewService struct {
	client *genai.Client
}

func NewReviewService(apiKey string) *ReviewService {
	logrus.Info("Creating Gemini AI client for code review")
	ctx := context.Background()
	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		logrus.WithError(err).Fatal("Failed to create Gemini client")
		panic(fmt.Sprintf("Failed to create Gemini client: %v", err))
	}

	logrus.Info("Gemini AI client created successfully")
	return &ReviewService{
		client: client,
	}
}

func (r *ReviewService) ReviewCode(changes []models.MRChange, title, description string, gitlabService *GitLabService, projectID int, targetBranch string) (*models.CodeReview, error) {
	logrus.WithFields(logrus.Fields{
		"changes_count": len(changes),
		"mr_title":      title,
	}).Info("Starting AI code review")

	ctx := context.Background()
	model := r.client.GenerativeModel("gemini-1.5-flash")

	model.SetTemperature(0.1)

	var codeContent strings.Builder
	codeContent.WriteString("## Merge Request Details\n")
	codeContent.WriteString(fmt.Sprintf("**Title:** %s\n", title))
	codeContent.WriteString(fmt.Sprintf("**Description:** %s\n\n", description))

	logrus.Debug("Building code content for AI review")

	processedFiles := 0
	for _, change := range changes {
		if change.DeletedFile {
			logrus.WithField("file", change.OldPath).Debug("Skipping deleted file")
			continue
		}

		logrus.WithFields(logrus.Fields{
			"file":     change.NewPath,
			"new_file": change.NewFile,
			"renamed":  change.RenamedFile,
		}).Debug("Processing file change")

		codeContent.WriteString(fmt.Sprintf("## File: %s\n", change.NewPath))
		if change.NewFile {
			codeContent.WriteString("(New file)\n")
		}
		if change.RenamedFile {
			codeContent.WriteString(fmt.Sprintf("(Renamed from: %s)\n", change.OldPath))
		}

		// Process the diff to add line numbers for AI reference
		codeContent.WriteString("```diff\n")
		processedDiff := r.addLineNumbersToDiff(change.Diff)
		codeContent.WriteString(processedDiff)
		codeContent.WriteString("\n```\n\n")
		processedFiles++
	}

	logrus.WithField("processed_files", processedFiles).Debug("Finished processing file changes")

	// Fetch custom review guidance from the repository
	guidance, err := gitlabService.GetReviewGuidance(projectID, targetBranch)
	if err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{
			"project_id": projectID,
			"branch":     targetBranch,
		}).Warn("Failed to fetch custom review guidance, using default")
		guidance = "" // Use default guidance
	}

	// Build the prompt with custom or default guidance
	var prompt string
	if guidance != "" {
		logrus.WithFields(logrus.Fields{
			"project_id":      projectID,
			"guidance_length": len(guidance),
		}).Info("Using custom review guidance from repository")
		
		prompt = fmt.Sprintf(`You are an expert code reviewer. Please review the following merge request changes according to the custom guidance provided below.

CUSTOM REVIEW GUIDANCE:
%s

IMPORTANT: Only comment on lines that are actually visible in the diff below. Do not reference line numbers outside of the changes shown.

Please format your response as follows:
- Start with a summary paragraph
- Then provide specific comments in this EXACT format:
  COMMENT:filename.go:diff_line_number:line_type:comment_text
  
  Where:
  - filename.go is the file path (exactly as shown in the diff)
  - diff_line_number is the DIFF_LINE number shown in brackets (e.g., if you see [DIFF_LINE:5,NEW_LINE:42], use 5)
  - line_type is either "new" (for lines starting with +), "old" (for lines starting with -), or "context" (for lines starting with space)
  - comment_text is your feedback
  
  Example: If you see "+ const myVar = 5 [DIFF_LINE:3,NEW_LINE:42]", use: COMMENT:src/main.go:3:new:This variable should be declared as const since it never changes.

CRITICAL: 
- Only use DIFF_LINE numbers from the brackets in the diff
- Only comment on lines that are actually changed or shown in the diff context
- Do not try to comment on lines outside the diff
- Follow the custom guidance provided above

Here are the changes to review:

%s

Be constructive and specific in your feedback. Only reference lines that are visible in the diff above.`, guidance, codeContent.String())
	} else {
		logrus.WithField("project_id", projectID).Info("Using default review guidance")
		
		prompt = fmt.Sprintf(`You are an expert code reviewer. Please review the following merge request changes and provide:

1. A brief summary of the changes
2. Specific actionable feedback for improvements with exact file and line references
3. Identify potential bugs, security issues, or performance problems
4. Suggest best practices if applicable

IMPORTANT: Only comment on lines that are actually visible in the diff below. Do not reference line numbers outside of the changes shown.

Please format your response as follows:
- Start with a summary paragraph
- Then provide specific comments in this EXACT format:
  COMMENT:filename.go:diff_line_number:line_type:comment_text
  
  Where:
  - filename.go is the file path (exactly as shown in the diff)
  - diff_line_number is the DIFF_LINE number shown in brackets (e.g., if you see [DIFF_LINE:5,NEW_LINE:42], use 5)
  - line_type is either "new" (for lines starting with +), "old" (for lines starting with -), or "context" (for lines starting with space)
  - comment_text is your feedback
  
  Example: If you see "+ const myVar = 5 [DIFF_LINE:3,NEW_LINE:42]", use: COMMENT:src/main.go:3:new:This variable should be declared as const since it never changes.

CRITICAL: 
- Only use DIFF_LINE numbers from the brackets in the diff
- Only comment on lines that are actually changed or shown in the diff context
- Do not try to comment on lines outside the diff

Here are the changes to review:

%s

Focus on:
- Code quality and maintainability
- Security vulnerabilities
- Performance issues
- Best practices
- Potential bugs
- Documentation needs

Be constructive and specific in your feedback. Only reference lines that are visible in the diff above.`, codeContent.String())
	}

	logrus.Debug("Sending request to Gemini AI for code review")
	resp, err := model.GenerateContent(ctx, genai.Text(prompt))
	if err != nil {
		logrus.WithError(err).Error("Failed to generate AI code review")
		return nil, fmt.Errorf("failed to generate review: %w", err)
	}

	if len(resp.Candidates) == 0 {
		logrus.Error("No response candidates generated by AI")
		return nil, fmt.Errorf("no response generated")
	}

	reviewText := ""
	for _, part := range resp.Candidates[0].Content.Parts {
		if txt, ok := part.(genai.Text); ok {
			reviewText += string(txt)
		}
	}

	logrus.WithField("review_length", len(reviewText)).Info("AI code review generated successfully")
	return r.parseReview(reviewText), nil
}

func (r *ReviewService) parseReview(reviewText string) *models.CodeReview {
	logrus.Debug("Parsing AI review response")
	lines := strings.Split(reviewText, "\n")

	var summary strings.Builder
	var comments []string
	var positionedComments []models.PositionedComment

	inSummary := true

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "COMMENT:") {
			inSummary = false
			comment := strings.TrimPrefix(line, "COMMENT:")
			comment = strings.TrimSpace(comment)

			if comment != "" {
				// Try to parse positioned comment format: filename:line:type:comment
				parts := strings.SplitN(comment, ":", 4)
				if len(parts) == 4 {
					filePath := parts[0]
					lineNumStr := parts[1]
					lineType := parts[2]
					commentText := parts[3]

					if lineNum, err := strconv.Atoi(lineNumStr); err == nil {
						// Valid positioned comment
						positionedComment := models.PositionedComment{
							FilePath:     filePath,
							LineNumber:   lineNum,
							LineType:     lineType,
							Comment:      commentText,
							OriginalLine: "", // We could enhance this later
						}
						positionedComments = append(positionedComments, positionedComment)

						logrus.WithFields(logrus.Fields{
							"file_path":   filePath,
							"line_number": lineNum,
							"line_type":   lineType,
						}).Debug("Parsed positioned comment")
					} else {
						// Fallback to general comment
						comments = append(comments, comment)
						logrus.WithField("comment", comment).Debug("Failed to parse line number, using general comment")
					}
				} else {
					// Fallback to general comment
					comments = append(comments, comment)
					logrus.WithField("comment", comment).Debug("Comment not in positioned format, using general comment")
				}
			}
		} else if inSummary && line != "" {
			if summary.Len() > 0 {
				summary.WriteString(" ")
			}
			summary.WriteString(line)
		}
	}

	logrus.WithFields(logrus.Fields{
		"summary_length":            len(summary.String()),
		"general_comments_count":    len(comments),
		"positioned_comments_count": len(positionedComments),
	}).Debug("Review parsing completed")

	return &models.CodeReview{
		Summary:            summary.String(),
		Comments:           comments,
		PositionedComments: positionedComments,
	}
}

func (r *ReviewService) addLineNumbersToDiff(diff string) string {
	lines := strings.Split(diff, "\n")
	var result strings.Builder
	diffLineNum := 0
	oldLineNum := 0
	newLineNum := 0

	for _, line := range lines {
		if strings.HasPrefix(line, "@@") {
			// Parse hunk header to get starting line numbers
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
			result.WriteString(line + "\n")
		} else if strings.HasPrefix(line, "+") {
			newLineNum++
			diffLineNum++
			result.WriteString(fmt.Sprintf("%s [DIFF_LINE:%d,NEW_LINE:%d]\n", line, diffLineNum, newLineNum))
		} else if strings.HasPrefix(line, "-") {
			oldLineNum++
			diffLineNum++
			result.WriteString(fmt.Sprintf("%s [DIFF_LINE:%d,OLD_LINE:%d]\n", line, diffLineNum, oldLineNum))
		} else if strings.HasPrefix(line, " ") {
			oldLineNum++
			newLineNum++
			diffLineNum++
			result.WriteString(fmt.Sprintf("%s [DIFF_LINE:%d,CONTEXT:%d]\n", line, diffLineNum, newLineNum))
		} else {
			result.WriteString(line + "\n")
		}
	}

	return result.String()
}
