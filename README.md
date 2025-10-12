# WhyTho: Making your code less questionable, one PR at a time

An AI-powered GitLab merge request reviewer bot that uses Google's Gemini LLM to provide intelligent code reviews. The bot automatically analyzes merge requests and posts constructive feedback as comments.

## Features

- 🤖 **AI-Powered Reviews**: Uses Google Gemini to analyze code changes and provide intelligent feedback
- 🔗 **GitLab Integration**: Seamless integration with GitLab webhooks
- 🚀 **Automatic Comments**: Posts review comments directly on merge requests
- 📍 **Positioned Comments**: AI can comment on specific lines in diffs for precise feedback
- 📋 **Custom Review Guidance**: Supports repository-specific review criteria via .whytho/guidance.md files
- 🔒 **Secure**: Supports webhook signature verification
- 📊 **Comprehensive Analysis**: Reviews code quality, security, performance, and best practices
- 📝 **Structured Logging**: Uses logrus for comprehensive structured logging
- 🐳 **Containerized**: Ready-to-deploy Docker setup

## Prerequisites

- Go 1.21 or later
- GitLab access token with API permissions
- Google Gemini API key
- Docker (optional, for containerized deployment)

## Setup

### 1. Clone the Repository

```bash
git clone https://github.com/vinamra28/whytho.git
cd whytho
```

### 2. Configure Environment Variables

Copy the example environment file and fill in your credentials:

```bash
cp .env.example .env
```

Edit `.env` with your configuration:

```env
GITLAB_TOKEN=your_gitlab_access_token_here
GITLAB_BASE_URL=https://gitlab.com
GEMINI_API_KEY=your_gemini_api_key_here
WEBHOOK_SECRET=your_webhook_secret_here
PORT=8080
```

### 3. Install Dependencies

```bash
go mod tidy
```

### 4. Run the Application

#### Option A: Direct Go Run

```bash
go run cmd/main.go
```

#### Option B: Docker Compose

```bash
docker-compose up --build
```

#### Option C: Docker Build

```bash
docker build -t whytho .
docker run -p 8080:8080 --env-file .env whytho
```

## GitLab Webhook Configuration

1. Go to your GitLab project/group settings
2. Navigate to **Webhooks**
3. Add a new webhook with:
   - **URL**: `http://your-server:8080/webhook`
   - **Secret Token**: Your `WEBHOOK_SECRET` value
   - **Trigger**: Select "Merge request events"
   - **SSL Verification**: Enable if using HTTPS

## How It Works

1. GitLab sends a webhook when a merge request is opened, reopened, or updated
2. The bot validates the webhook signature (if configured)
3. Fetches the merge request changes via GitLab API
4. Attempts to fetch custom review guidance from `.whytho/guidance.md` in the target repository
5. Sends the code changes to Google Gemini for analysis with custom or default guidance
6. Posts AI-generated review comments back to the merge request (both general and line-specific positioned comments)

## API Endpoints

- `POST /webhook` - GitLab webhook endpoint
- `GET /health` - Health check endpoint

## Project Structure

```tree
├── cmd/
│   └── main.go                 # Application entry point
├── internal/
│   ├── config/
│   │   └── config.go          # Configuration management
│   ├── handlers/
│   │   └── webhook.go         # Webhook handlers
│   ├── models/
│   │   └── models.go          # Data structures
│   ├── server/
│   │   └── server.go          # HTTP server setup
│   └── services/
│       ├── gitlab.go          # GitLab API client
│       └── review.go          # Gemini AI integration
├── Dockerfile                 # Docker configuration
├── docker-compose.yml         # Docker Compose setup
├── go.mod                     # Go module definition
└── README.md                  # This file
```

## Custom Review Guidance

The bot supports repository-specific review criteria by reading a `.whytho/guidance.md` file from the target repository. This allows each project to customize the AI reviewer's focus areas and criteria.

### Creating .whytho/guidance.md

Create a `.whytho/guidance.md` file in your repository with your custom review guidelines:

```markdown
# Code Review Guidance

## Review Focus Areas

### 1. Go-Specific Best Practices

- Ensure proper error handling with explicit error returns
- Check for potential goroutine leaks
- Verify proper use of context.Context for cancellation

### 2. Security Considerations

- Never log sensitive information (tokens, passwords, secrets)
- Validate all input parameters
- Use secure defaults for configuration

### 3. Performance and Reliability

- Look for potential memory leaks
- Check for inefficient loops or operations
- Validate retry logic and backoff strategies
```

### Fallback Behavior

If no `.whytho/guidance.md` file is found, the bot uses comprehensive default review criteria covering:

- Code quality and maintainability
- Security vulnerabilities
- Performance issues
- Best practices
- Potential bugs
- Documentation needs

## Path Exclusion Configuration

The bot supports excluding specific files and directories from review using a `.whytho/config.yaml` file in your repository.

### Creating .whytho/config.yaml

Create a `.whytho/config.yaml` file in your repository root to specify which paths should be excluded from review:

```yaml
excludePaths:
  - "vendor/**"
  - "*.generated.go"
  - "docs/**"
  - "test/fixtures/**"
```

### Supported Path Patterns

- **Exact matches**: `vendor/module.go`
- **Glob patterns**: `*.go`, `test_*.py`
- **Directory exclusions**: `vendor/**` (excludes all files in vendor directory and subdirectories)
- **File extensions**: `*.min.js`, `*.generated.*`

### Configuration Priority

The bot checks for `.whytho/config.yaml` in the following order:

1. **Modified in MR**: If the config file is changed in the current merge request, uses the new version
2. **Target branch**: If not modified, fetches the config from the target branch (e.g., `main`)
3. **Fallback**: If no config file exists, reviews all files

### Example Configuration

```yaml
excludePaths:
  - "vendor/**" # Exclude all vendor dependencies
  - "*.pb.go" # Exclude generated protobuf files
  - "*.generated.go" # Exclude all generated Go files
  - "docs/**" # Exclude documentation directory
  - "test/fixtures/**" # Exclude test fixtures
  - "*.min.js" # Exclude minified JavaScript
  - "migrations/**" # Exclude database migrations
```

### Logging

When files are excluded, the bot logs:

- Number of excluded files
- List of excluded file paths
- Remaining files for review

If all files are excluded, the bot posts a summary comment explaining the exclusion.

## Required Tokens and Permissions

### GitLab Access Token

Create a GitLab access token with the following scopes:

- `api` - Full API access
- `read_api` - Read API access
- `read_repository` - Read repository access

### Gemini API Key

1. Go to [Google AI Studio](https://makersuite.google.com/app/apikey)
2. Create a new API key
3. Use this key as your `GEMINI_API_KEY`

## Security Considerations

- Always use HTTPS in production
- Set a strong `WEBHOOK_SECRET` for webhook verification
- Keep your GitLab token and Gemini API key secure
- Consider rate limiting for the webhook endpoint
- Run the application behind a reverse proxy (nginx, etc.)

## Troubleshooting

### Common Issues

1. **Webhook not triggering**: Check that the webhook URL is accessible and the secret matches
2. **API errors**: Verify your GitLab token has the required permissions
3. **Gemini errors**: Ensure your API key is valid and you have sufficient quota
4. **Connection issues**: Check network connectivity and firewall settings

### Logs

The application uses structured JSON logging via logrus and logs important events including:

- Webhook received events
- GitLab API calls (merge request changes, posting comments)
- Gemini API interactions
- Custom guidance fetching from repositories
- Positioned comment processing
- Error conditions and fallback behaviors

Logs include structured fields for easy filtering and monitoring:

```json
{
  "level": "info",
  "msg": "Code review completed",
  "project_id": 123,
  "mr_iid": 45,
  "general_comments_count": 2,
  "positioned_comments_count": 5,
  "time": "2025-01-15T10:30:00Z"
}
```

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests if applicable
5. Submit a pull request

## License

This project is licensed under the MIT License. See the LICENSE file for details.

## Support

For issues and questions:

1. Check the troubleshooting section
2. Open an issue on GitHub
3. Check GitLab and Gemini API documentation
