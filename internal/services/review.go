package services

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/vinamra28/whytho/internal/models"
	"google.golang.org/genai"
)

type ReviewService struct {
	client *genai.Client
}

func NewReviewService(apiKey string) *ReviewService {
	logrus.Info("Creating Gemini AI client for code review")
	ctx := context.Background()
	config := &genai.ClientConfig{
		APIKey: apiKey,
	}
	client, err := genai.NewClient(ctx, config)
	if err != nil {
		logrus.WithError(err).Fatal("Failed to create Gemini client")
		panic(fmt.Sprintf("Failed to create Gemini client: %v", err))
	}

	logrus.Info("Gemini AI client created successfully")
	return &ReviewService{
		client: client,
	}
}

func (r *ReviewService) ReviewCode(changes []models.MRChange, title, description string, gitlabService *GitLabService, projectID, mrIID int, targetBranch string) (*models.CodeReview, error) {
	logrus.WithFields(logrus.Fields{
		"changes_count": len(changes),
		"mr_title":      title,
	}).Info("Starting AI code review")

	ctx := context.Background()

	// Fetch WhyTho config to filter excluded paths
	whyThoConfig, err := gitlabService.GetWhyThoConfig(projectID, mrIID, targetBranch, changes)
	if err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{
			"project_id": projectID,
			"mr_iid":     mrIID,
		}).Warn("Failed to fetch WhyTho config, proceeding without path filtering")
		whyThoConfig = &models.WhyThoConfig{ExcludePaths: []string{}}
	}

	// Filter out excluded paths
	filteredChanges, excludedFiles := r.filterExcludedChanges(changes, whyThoConfig)

	if len(excludedFiles) > 0 {
		logrus.WithFields(logrus.Fields{
			"project_id":       projectID,
			"mr_iid":           mrIID,
			"excluded_count":   len(excludedFiles),
			"excluded_files":   excludedFiles,
			"total_changes":    len(changes),
			"filtered_changes": len(filteredChanges),
		}).Info("Excluded files from review based on WhyTho config")
	}

	if len(filteredChanges) == 0 {
		logrus.WithFields(logrus.Fields{
			"project_id":     projectID,
			"mr_iid":         mrIID,
			"excluded_count": len(excludedFiles),
		}).Info("All files excluded from review, returning empty review")

		summary := "All files in this merge request are excluded from review based on the .whytho/config.yaml configuration."
		if len(excludedFiles) > 0 {
			summary += fmt.Sprintf(" Excluded files: %s", strings.Join(excludedFiles, ", "))
		}

		return &models.CodeReview{
			Summary:            summary,
			Comments:           []string{},
			PositionedComments: []models.PositionedComment{},
		}, nil
	}

	var codeContent strings.Builder
	codeContent.WriteString("## Merge Request Details\n")
	codeContent.WriteString(fmt.Sprintf("**Title:** %s\n", title))
	codeContent.WriteString(fmt.Sprintf("**Description:** %s\n\n", description))

	if len(excludedFiles) > 0 {
		codeContent.WriteString(fmt.Sprintf("## Excluded Files\nThe following files were excluded from review based on .whytho/config.yaml: %s\n\n", strings.Join(excludedFiles, ", ")))
	}

	logrus.Debug("Building code content for AI review")

	processedFiles := 0
	for _, change := range filteredChanges {
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

		prompt = fmt.Sprintf(`You are an expert code reviewer with deep knowledge of software engineering best practices. Review the following merge request changes according to the custom guidance provided below.

CUSTOM REVIEW GUIDANCE:
%s

IMPORTANT: Only comment on lines that are actually visible in the diff below. Do not reference line numbers outside of the changes shown.

When providing feedback, be solution-oriented and include specific improvement suggestions:
- For code quality issues: Suggest better patterns, refactoring opportunities, or architectural improvements
- For performance concerns: Provide specific optimization techniques or alternative approaches
- For security issues: Recommend secure coding practices and specific fixes
- For maintainability: Suggest ways to make code more readable, testable, or modular

Please format your response as follows:
- Start with a summary paragraph highlighting the most important findings
- Then provide specific comments in this EXACT format:
  COMMENT:filename.go:diff_line_number:line_type:severity:comment_text
  
  Where:
  - filename.go is the file path (exactly as shown in the diff)
  - diff_line_number is the DIFF_LINE number shown in brackets (e.g., if you see [DIFF_LINE:5,NEW_LINE:42], use 5)
  - line_type is either "new" (for lines starting with +), "old" (for lines starting with -), or "context" (for lines starting with space)
  - severity is one of: LOW, MEDIUM, HIGH, or CRITICAL
  - comment_text is your detailed feedback with specific suggestions
  
  Comment Structure Guidelines:
  - Start with a clear problem statement
  - Provide specific improvement suggestion
  - Include code examples when helpful (use markdown code blocks)
  - Explain the benefits of the suggested change
  - Reference relevant best practices or patterns
  
  Severity Guidelines:
  - CRITICAL: Security vulnerabilities, potential data loss, system crashes
  - HIGH: Major bugs, performance issues, serious logic errors
  - MEDIUM: Code quality issues, maintainability concerns, minor bugs
  - LOW: Style improvements, documentation suggestions, minor optimizations
  
  Example: COMMENT:src/main.go:3:new:MEDIUM:Consider using a more specific variable name and declaring it as const for better readability and immutability. Suggestion: "const maxRetryCount = 5" instead of "myVar = 5". This makes the purpose clear and prevents accidental modification.

CRITICAL: 
- Only use DIFF_LINE numbers from the brackets in the diff
- Only comment on lines that are actually changed or shown in the diff context
- Do not try to comment on lines outside the diff
- Follow the custom guidance provided above
- Always provide actionable suggestions with clear reasoning

Here are the changes to review:

%s

Focus on providing constructive, actionable feedback that helps improve code quality, security, and maintainability.`, guidance, codeContent.String())
	} else {
		logrus.WithField("project_id", projectID).Info("Using default review guidance")

		prompt = fmt.Sprintf(`You are an expert code reviewer with deep knowledge of software engineering best practices. Analyze the following merge request changes and provide comprehensive, actionable feedback.

REVIEW OBJECTIVES:
1. Identify code quality improvements and refactoring opportunities
2. Suggest performance optimizations and architectural enhancements
3. Highlight security vulnerabilities and recommend secure coding practices
4. Propose maintainability improvements and best practices
5. Detect potential bugs and logic errors

IMPORTANT: Only comment on lines that are actually visible in the diff below. Do not reference line numbers outside of the changes shown.

When providing feedback, be solution-oriented and include specific improvement suggestions:
- For code quality issues: Suggest better patterns, refactoring opportunities, or architectural improvements
- For performance concerns: Provide specific optimization techniques or alternative approaches
- For security issues: Recommend secure coding practices and specific fixes
- For maintainability: Suggest ways to make code more readable, testable, or modular

Please format your response as follows:
- Start with a summary paragraph highlighting the most important findings and overall assessment
- Then provide specific comments in this EXACT format:
  COMMENT:filename.go:diff_line_number:line_type:severity:comment_text
  
  Where:
  - filename.go is the file path (exactly as shown in the diff)
  - diff_line_number is the DIFF_LINE number shown in brackets (e.g., if you see [DIFF_LINE:5,NEW_LINE:42], use 5)
  - line_type is either "new" (for lines starting with +), "old" (for lines starting with -), or "context" (for lines starting with space)
  - severity is one of: LOW, MEDIUM, HIGH, or CRITICAL
  - comment_text is your detailed feedback with specific suggestions
  
  Comment Structure Guidelines:
  - Start with a clear problem statement
  - Provide specific improvement suggestion with reasoning
  - Include code examples when helpful (use markdown code blocks)
  - Explain the benefits of the suggested change
  - Reference relevant best practices, design patterns, or standards
  
  Severity Guidelines:
  - CRITICAL: Security vulnerabilities, potential data loss, system crashes, major logic errors
  - HIGH: Performance bottlenecks, significant bugs, architectural issues, race conditions
  - MEDIUM: Code quality issues, maintainability concerns, minor bugs, suboptimal patterns
  - LOW: Style improvements, documentation suggestions, minor optimizations, naming conventions
  
  Example: COMMENT:src/main.go:3:new:MEDIUM:Consider using a more descriptive variable name and declaring it as const for better readability and immutability. Suggestion: Replace "myVar = 5" with "const maxRetryCount = 5". This improves code clarity and prevents accidental modification, following Go naming conventions.

CRITICAL: 
- Only use DIFF_LINE numbers from the brackets in the diff
- Only comment on lines that are actually changed or shown in the diff context
- Do not try to comment on lines outside the diff
- Always provide actionable suggestions with clear reasoning
- Include specific code examples when proposing alternatives

Here are the changes to review:

%s

Focus on providing constructive, actionable feedback that helps developers write better, more secure, and maintainable code.`, codeContent.String())
	}

	logrus.Debug("Sending request to Gemini AI for code review")
	content := genai.NewContentFromText(prompt, "user")
	config := &genai.GenerateContentConfig{
		Temperature: genai.Ptr(float32(0.1)),
	}
	resp, err := r.client.Models.GenerateContent(ctx, "gemini-2.5-pro", []*genai.Content{content}, config)
	if err != nil {
		logrus.WithError(err).Error("Failed to generate AI code review")
		return nil, fmt.Errorf("failed to generate review: %w", err)
	}

	if len(resp.Candidates) == 0 {
		logrus.Error("No response candidates generated by AI")
		return nil, fmt.Errorf("no response generated")
	}

	reviewText := resp.Text()

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
				// Try to parse positioned comment format: filename:line:type:severity:comment
				parts := strings.SplitN(comment, ":", 5)
				if len(parts) == 5 {
					filePath := parts[0]
					lineNumStr := parts[1]
					lineType := parts[2]
					severity := parts[3]
					commentText := parts[4]

					if lineNum, err := strconv.Atoi(lineNumStr); err == nil {
						// Valid positioned comment
						positionedComment := models.PositionedComment{
							FilePath:     filePath,
							LineNumber:   lineNum,
							LineType:     lineType,
							Severity:     severity,
							Comment:      commentText,
							OriginalLine: "", // We could enhance this later
						}
						positionedComments = append(positionedComments, positionedComment)

						logrus.WithFields(logrus.Fields{
							"file_path":   filePath,
							"line_number": lineNum,
							"line_type":   lineType,
							"severity":    severity,
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

func (r *ReviewService) shouldExcludePath(filePath string, excludePaths []string) bool {
	for _, pattern := range excludePaths {
		// Use filepath.Match for basic glob pattern matching
		matched, err := filepath.Match(pattern, filePath)
		if err != nil {
			logrus.WithError(err).WithFields(logrus.Fields{
				"file_path": filePath,
				"pattern":   pattern,
			}).Warn("Invalid exclude pattern, skipping")
			continue
		}
		if matched {
			return true
		}

		// Also check if the pattern with ** matches (simple directory prefix matching)
		if strings.HasSuffix(pattern, "/**") {
			prefix := strings.TrimSuffix(pattern, "/**")
			if strings.HasPrefix(filePath, prefix+"/") || filePath == prefix {
				return true
			}
		}
	}
	return false
}

func (r *ReviewService) filterExcludedChanges(changes []models.MRChange, config *models.WhyThoConfig) ([]models.MRChange, []string) {
	if config == nil || len(config.ExcludePaths) == 0 {
		return changes, []string{}
	}

	var filteredChanges []models.MRChange
	var excludedFiles []string

	for _, change := range changes {
		filePath := change.NewPath
		if filePath == "" {
			filePath = change.OldPath // For deleted files
		}

		if r.shouldExcludePath(filePath, config.ExcludePaths) {
			excludedFiles = append(excludedFiles, filePath)
			logrus.WithFields(logrus.Fields{
				"file_path":     filePath,
				"exclude_paths": config.ExcludePaths,
			}).Debug("Excluding file from review based on WhyTho config")
		} else {
			filteredChanges = append(filteredChanges, change)
		}
	}

	return filteredChanges, excludedFiles
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
