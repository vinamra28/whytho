package models

type GitLabWebhook struct {
	ObjectKind       string           `json:"object_kind"`
	EventType        string           `json:"event_type"`
	User             User             `json:"user"`
	Project          Project          `json:"project"`
	ObjectAttributes ObjectAttributes `json:"object_attributes"`
	Repository       Repository       `json:"repository"`
}

type User struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Username string `json:"username"`
	Email    string `json:"email"`
}

type Project struct {
	ID                int    `json:"id"`
	Name              string `json:"name"`
	Description       string `json:"description"`
	WebURL            string `json:"web_url"`
	AvatarURL         string `json:"avatar_url"`
	GitSSHURL         string `json:"git_ssh_url"`
	GitHTTPURL        string `json:"git_http_url"`
	Namespace         string `json:"namespace"`
	VisibilityLevel   int    `json:"visibility_level"`
	PathWithNamespace string `json:"path_with_namespace"`
	DefaultBranch     string `json:"default_branch"`
	Homepage          string `json:"homepage"`
	URL               string `json:"url"`
	SSHURL            string `json:"ssh_url"`
	HTTPURL           string `json:"http_url"`
}

type ObjectAttributes struct {
	ID              int    `json:"id"`
	IID             int    `json:"iid"`
	Title           string `json:"title"`
	Description     string `json:"description"`
	State           string `json:"state"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
	TargetBranch    string `json:"target_branch"`
	SourceBranch    string `json:"source_branch"`
	SourceProjectID int    `json:"source_project_id"`
	TargetProjectID int    `json:"target_project_id"`
	AuthorID        int    `json:"author_id"`
	AssigneeID      int    `json:"assignee_id"`
	URL             string `json:"url"`
	Source          Source `json:"source"`
	Target          Target `json:"target"`
	LastCommit      Commit `json:"last_commit"`
	WorkInProgress  bool   `json:"work_in_progress"`
	Assignee        User   `json:"assignee"`
	Action          string `json:"action"`
}

type Source struct {
	Name            string `json:"name"`
	Description     string `json:"description"`
	WebURL          string `json:"web_url"`
	AvatarURL       string `json:"avatar_url"`
	GitSSHURL       string `json:"git_ssh_url"`
	GitHTTPURL      string `json:"git_http_url"`
	Namespace       string `json:"namespace"`
	VisibilityLevel int    `json:"visibility_level"`
	DefaultBranch   string `json:"default_branch"`
	Homepage        string `json:"homepage"`
	URL             string `json:"url"`
	SSHURL          string `json:"ssh_url"`
	HTTPURL         string `json:"http_url"`
}

type Target struct {
	Name            string `json:"name"`
	Description     string `json:"description"`
	WebURL          string `json:"web_url"`
	AvatarURL       string `json:"avatar_url"`
	GitSSHURL       string `json:"git_ssh_url"`
	GitHTTPURL      string `json:"git_http_url"`
	Namespace       string `json:"namespace"`
	VisibilityLevel int    `json:"visibility_level"`
	DefaultBranch   string `json:"default_branch"`
	Homepage        string `json:"homepage"`
	URL             string `json:"url"`
	SSHURL          string `json:"ssh_url"`
	HTTPURL         string `json:"http_url"`
}

type Commit struct {
	ID        string `json:"id"`
	Message   string `json:"message"`
	Timestamp string `json:"timestamp"`
	URL       string `json:"url"`
	Author    Author `json:"author"`
}

type Author struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type Repository struct {
	Name        string `json:"name"`
	URL         string `json:"url"`
	Description string `json:"description"`
	Homepage    string `json:"homepage"`
}

type MRChange struct {
	OldPath     string `json:"old_path"`
	NewPath     string `json:"new_path"`
	AMode       string `json:"a_mode"`
	BMode       string `json:"b_mode"`
	NewFile     bool   `json:"new_file"`
	RenamedFile bool   `json:"renamed_file"`
	DeletedFile bool   `json:"deleted_file"`
	Diff        string `json:"diff"`
}

type CodeReview struct {
	Summary            string              `json:"summary"`
	Comments           []string            `json:"comments"`
	PositionedComments []PositionedComment `json:"positioned_comments"`
}

type PositionedComment struct {
	FilePath     string `json:"file_path"`
	LineNumber   int    `json:"line_number"`
	LineType     string `json:"line_type"` // "old", "new", or "context"
	Severity     string `json:"severity"`   // "LOW", "MEDIUM", "HIGH", or "CRITICAL"
	Comment      string `json:"comment"`
	OriginalLine string `json:"original_line"`
	LineCode     string `json:"line_code"` // GitLab's line code for positioning
}

type DiffLine struct {
	Type       string `json:"type"`         // "+", "-", " " (context)
	Content    string `json:"content"`      // The actual line content
	OldLineNum int    `json:"old_line_num"` // Line number in old file (0 if new line)
	NewLineNum int    `json:"new_line_num"` // Line number in new file (0 if deleted line)
	Position   int    `json:"position"`     // Position in diff for GitLab API
}
