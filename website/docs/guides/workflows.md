---
sidebar_position: 8
---

# Common Workflows

Practical workflows for using SCM effectively in your daily development.

## Getting Started Workflow

### 1. Initialize SCM

```bash
# In your project directory
scm init

# Or initialize globally
scm init --home
```

### 2. Discover and Add Bundles

```bash
# Find relevant bundles
scm remote discover golang

# Add a remote
scm remote add community alice/scm-golang

# Browse what's available
scm remote browse community

# Pull bundles you want
scm fragment install community/go-development
scm fragment install community/testing-patterns
```

### 3. Create a Profile

```bash
# Create a development profile
scm profile create go-dev \
  -b go-development \
  -b testing-patterns \
  -d "Go development environment"

# Set as default
scm remote default go-dev
```

### 4. Start Coding

```bash
# Your context is now automatically injected
scm run  # or just start Claude Code
```

## Daily Development Workflow

### Morning Setup

```bash
# Sync any remote updates
scm remote sync

# Check your current profile
scm profile show default
```

### During Development

Your context is automatically available. For specific tasks:

```bash
# Add security context for a security review
scm run -f security#fragments/owasp "review this authentication code"

# Use a specific profile for frontend work
scm run -p frontend-dev "help with React component"

# Preview what context will be used
scm run --dry-run --print
```

### End of Day

```bash
# If you created new fragments, commit them
git add .scm/
git commit -m "Update SCM configuration"
```

## Team Onboarding Workflow

### For Team Leads

1. **Create team bundles repository**:

```bash
mkdir team-scm && cd team-scm
mkdir -p scm/v1/bundles scm/v1/profiles
```

2. **Add team standards**:

```yaml
# scm/v1/bundles/team-standards.yaml
version: "1.0"
description: Team coding standards
fragments:
  code-style:
    content: |
      # Team Code Style
      - Use gofmt for all Go code
      - 100 character line limit
      - Descriptive variable names
```

3. **Create team profile**:

```yaml
# scm/v1/profiles/team-developer.yaml
description: Standard team development environment
bundles:
  - team-standards
  - security-basics
```

4. **Publish**:

```bash
git init && git add . && git commit -m "Initial team SCM"
git remote add origin https://github.com/myorg/scm-team.git
git push -u origin main
```

### For New Team Members

```bash
# Add team remote
scm remote add team myorg/scm-team

# Sync team bundles
scm remote sync

# Use team profile
scm profile create my-dev --parent team/team-developer
scm profile default my-dev
```

## Project-Specific Workflow

### Setting Up a New Project

```bash
cd my-project
scm init

# Create project-specific profile
scm profile create project \
  --parent go-dev \
  -b project-specific \
  -d "This project's development context"

scm profile default project
```

### Project Bundle

Create a bundle specific to your project:

```yaml
# .scm/bundles/project-specific.yaml
version: "1.0"
description: Project-specific context

fragments:
  architecture:
    content: |
      # Project Architecture

      This project uses:
      - Clean architecture with domain/usecase/infrastructure layers
      - PostgreSQL for persistence
      - Redis for caching
      - gRPC for internal services

  conventions:
    content: |
      # Project Conventions

      - All handlers in internal/handlers/
      - Domain models in internal/domain/
      - Use structured logging with zap
```

## Multi-Language Workflow

### Switching Contexts

```bash
# Create language-specific profiles
scm profile create go-work -b go-development -b go-testing
scm profile create python-work -b python-development -b python-testing
scm profile create frontend-work -b typescript -b react

# Switch based on current task
scm profile default go-work      # Working on Go
scm profile default python-work  # Switching to Python
```

### Per-Directory Configuration

Use different `.scm/` configurations in different project directories:

```
~/projects/
├── go-api/
│   └── .scm/
│       └── profiles/default.yaml  # Go-focused
├── python-ml/
│   └── .scm/
│       └── profiles/default.yaml  # Python/ML-focused
└── react-app/
    └── .scm/
        └── profiles/default.yaml  # Frontend-focused
```

## Security Review Workflow

### Setup

```bash
# Add security bundles
scm fragment install scm-main/security
scm fragment install scm-main/owasp
```

### Conducting Reviews

```bash
# General security review
scm run -t security "review this code for security issues"

# OWASP-focused review
scm run -f security#fragments/owasp-top-10 "check for OWASP top 10 vulnerabilities"

# Authentication-specific
scm run -f security#fragments/auth-patterns "review authentication implementation"
```

## Code Review Workflow

### Preparing Context

```bash
# Create a code review profile
scm profile create reviewer \
  -b code-quality \
  -b testing-patterns \
  -b security-basics \
  -d "Code review context"
```

### During Review

```bash
# Use review profile
scm run -p reviewer "review this PR for code quality"

# Add specific concerns
scm run -p reviewer -f performance#fragments/optimization \
  "review for performance issues"
```

## CI/CD Integration Workflow

### In CI Pipeline

```yaml
# .github/workflows/ci.yml
jobs:
  lint:
    steps:
      - uses: actions/checkout@v4
      - name: Setup SCM
        run: |
          go install github.com/SophisticatedContextManager/scm@latest
          scm remote sync

      - name: AI Code Review
        run: |
          scm run -p code-reviewer "review changes in this PR" \
            --output review.md
```

### Lockfile for Reproducibility

```bash
# Generate lockfile
scm remote lock

# Commit lockfile
git add .scm/lock.yaml
git commit -m "Lock SCM dependencies"
```

In CI:

```bash
# Install exact versions
scm remote install
```

## Troubleshooting Workflow

### When Context Isn't Working

```bash
# Check current configuration
scm profile show default

# Preview assembled context
scm run --dry-run --print

# Check hooks are applied
cat .claude/settings.json | jq '.hooks'

# Reapply hooks
scm hooks apply
```

### When Bundles Are Missing

```bash
# Check what's installed
scm fragment list

# Check what's available remotely
scm remote browse scm-main

# Sync missing dependencies
scm remote sync
```

## Tips and Best Practices

### Keep Context Focused

```bash
# Instead of one huge profile
scm profile create everything -b bundle1 -b bundle2 -b bundle3...

# Create task-specific profiles
scm profile create api-dev -b go-development -b api-patterns
scm profile create testing -b testing-patterns -b mocking
scm profile create security -b security -b owasp
```

### Use Tags Effectively

```yaml
# In your bundles
fragments:
  quick-reference:
    tags: [quick, cheatsheet]
    content: ...

  detailed-guide:
    tags: [detailed, learning]
    content: ...
```

```bash
# Quick reference only
scm run -t quick "remind me of the syntax"

# Detailed learning
scm run -t detailed "explain this concept"
```

### Version Control Your Configuration

```bash
# Always commit SCM configuration
git add .scm/
git commit -m "Update SCM configuration"
```

### Regular Maintenance

```bash
# Weekly: sync remote updates
scm remote sync

# Monthly: review and clean up profiles
scm profile list
scm fragment list

# As needed: update lockfile
scm remote lock
```
