---
sidebar_position: 4
---

# Profiles

A **profile** is a named configuration that references bundles, tags, and variables. Profiles enable quick context switching.

## Profile Structure

Profiles are stored in `.scm/profiles/` as YAML files:

```yaml
description: "Profile description"
default: false                      # Mark as default profile

parents:                            # Inherit from other profiles
  - base-profile
  - scm-main/python-developer

tags:                              # Include fragments with these tags
  - golang
  - testing

bundles:                           # Bundle references
  - go-development                 # Local bundle
  - scm-main/security             # Remote bundle
  - my-bundle#fragments/specific  # Specific fragment
  - my-bundle#prompts/review      # Specific prompt

variables:                         # Template variables (Mustache)
  DATABASE_URL: "postgresql://..."
  PROJECT_NAME: "my-app"
  DEBUG: "true"
```

## Content Reference Syntax

| Format | Description |
|--------|-------------|
| `bundle-name` | Entire bundle (all content) |
| `bundle#fragments/name` | Specific fragment |
| `bundle#prompts/name` | Specific prompt |
| `bundle#mcp` | All MCP servers from bundle |
| `bundle#mcp/name` | Specific MCP server |
| `remote/bundle` | Bundle from remote |
| `remote/bundle#fragments/x` | Fragment from remote |

### Extended Formats

| Format | Description |
|--------|-------------|
| `https://github.com/user/repo@v1/bundles/name` | Full URL with version |
| `git@github.com:user/repo#fragments/name` | Git SSH format |

## Using Profiles

```bash
# Run with a profile
scm run -p developer "implement error handling"

# Preview profile context
scm run -p developer --dry-run

# Use remote profile directly
scm run -p scm-main/python-developer "help with Python"

# Combine profile with extra fragments
scm run -p developer -f security#fragments/owasp "audit code"
```

## Managing Profiles

```bash
scm profile list                    # List all profiles
scm profile show developer          # Show profile details
scm profile create my-profile       # Create new profile
scm profile edit developer          # Edit in configured editor
scm profile delete old-profile      # Remove profile
scm profile install scm-main/dev    # Install from remote
```

### Create with Options

```bash
scm profile create backend \
  --parent base \
  --parent scm-main/security \
  -b go-development \
  -b testing \
  -d "Backend developer profile"
```

## Profile Inheritance

Profiles can inherit from other profiles using `parents`:

```yaml
# base.yaml
description: "Base configuration"
bundles:
  - core-standards
variables:
  LOG_LEVEL: "info"

# developer.yaml
description: "Developer profile"
parents:
  - base                    # Inherit from local
  - scm-main/security      # Inherit from remote
bundles:
  - dev-tools              # Add more bundles
variables:
  LOG_LEVEL: "debug"       # Override parent value
  DEV_MODE: "true"         # Add new variable
```

### Inheritance Rules

- **Order matters**: Later parents override earlier ones
- **Child overrides all**: Child values override all parent values
- **Bundles merge**: No duplicates
- **Tags merge**: Combined from all parents
- **Variables merge**: Child overrides parent values
- **Circular detection**: SCM errors on circular references

## Default Profiles

Mark a profile as default to load automatically:

```yaml
# .scm/profiles/developer.yaml
description: "Default dev profile"
default: true
bundles:
  - standards
```

Or in config.yaml:

```yaml
defaults:
  profiles:
    - developer
    - scm-main/base
```

## Variables

Profile variables are used in Mustache templates:

```yaml
# Profile
variables:
  PROJECT_NAME: "my-app"
  LANGUAGE: "Go"
  TEAM: "backend"
```

```yaml
# Fragment content using variables
content: |
  # {{PROJECT_NAME}} Development

  This {{LANGUAGE}} project is maintained by {{TEAM}}.
```

See [Templating](/guides/templating) for full variable documentation.

## Inline Profiles

Profiles can be defined directly in config.yaml:

```yaml
# .scm/config.yaml
profiles:
  quick-review:
    description: "Quick code review"
    bundles:
      - code-review
    variables:
      REVIEW_DEPTH: "surface"
```

Use like any other profile:

```bash
scm run -p quick-review "review this PR"
```
