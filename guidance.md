# Code Review Guidance

This file contains custom review guidance for this project. The AI reviewer will use these guidelines when reviewing merge requests.

## Review Focus Areas

### 1. Go-Specific Best Practices

- Ensure proper error handling with explicit error returns
- Check for potential goroutine leaks
- Verify proper use of context.Context for cancellation
- Look for missing defer statements for resource cleanup
- Ensure proper use of channels and avoid deadlocks

### 2. Kubernetes Operator Patterns

- Verify controller reconciliation logic is idempotent
- Check for proper status updates in Custom Resources
- Ensure proper finalizer handling for resource cleanup
- Validate RBAC permissions are minimal and specific
- Check for proper error handling in controller loops

### 3. Security Considerations

- Never log sensitive information (tokens, passwords, secrets)
- Validate all input parameters
- Use secure defaults for configuration
- Ensure proper RBAC and security contexts
- Check for SQL injection or similar vulnerabilities

### 4. Performance and Reliability

- Look for potential memory leaks
- Check for inefficient loops or operations
- Ensure proper resource limits and requests
- Validate retry logic and backoff strategies
- Check for proper timeouts on operations

### 5. Code Quality

- Ensure comprehensive error messages for debugging
- Check for proper logging at appropriate levels
- Validate test coverage for new functionality
- Ensure code follows Go conventions (gofmt, golint)
- Check for proper documentation and comments

### 6. Project-Specific Rules

- All API changes must be backward compatible
- New features must include unit tests
- Controllers must implement proper metrics
- All external dependencies must be approved
- Configuration changes require documentation updates

## Review Comments Format

When providing feedback, please:

- Be specific about the issue and suggest solutions
- Reference relevant documentation or best practices
- Prioritize security and reliability issues
- Be constructive and educational in tone
