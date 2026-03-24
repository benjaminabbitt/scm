---
sidebar_position: 2
---

# Quick Start

Get up and running with SCM in minutes.

## Initialize Your Project

```bash
# Create .scm directory in your project
scm init

# Or create a global config at ~/.scm
scm init --home
```

## Create Your First Bundle

```bash
# Create a new bundle
scm bundle create my-standards
```

This creates `.scm/bundles/my-standards.yaml` with a basic structure.

## Add Content to Your Bundle

Edit the bundle to add your context:

```yaml
version: "1.0.0"
description: "My coding standards"
tags:
  - development

fragments:
  coding-standards:
    tags: [style]
    content: |
      # My Coding Standards
      - Use meaningful variable names
      - Write tests for all new code
      - Document public APIs
```

## Run with Your Context

```bash
# Include your bundle when running AI
scm run -f my-standards "Help me with this code"

# Preview what context would be sent
scm run -n

# Use a profile for common combinations
scm run -p developer "implement error handling"
```

## Build Context On-the-Fly

You don't need to create profile files - assemble context dynamically with flags:

```bash
# Combine multiple bundles for a task
scm run -f go-development -f testing-patterns -f security \
  "implement user authentication with tests"

# Mix bundles and tags
scm run -f api-design -t best-practices "design the REST API"

# Pull specific fragments from bundles
scm run \
  -f go-development#fragments/error-handling \
  -f go-development#fragments/testing \
  "write error handling with tests"

# Use remote bundles without installing
scm run -f scm-main/security#fragments/owasp "security review"
```

### Preview Before Running

```bash
# See what context would be assembled
scm run -f go-development -f security --dry-run

# See the actual content
scm run -f go-development -f security --dry-run --print
```

## Discover and Use Community Bundles

```bash
# Find SCM repositories
scm remote discover golang

# Add a remote
scm remote add community alice/scm-golang

# Use remote content directly
scm run -f community/go-testing "help with tests"

# Or pull for local use
scm fragment install community/go-testing
```

## Next Steps

- [Ad-Hoc Context Assembly](/guides/adhoc-context) - Build faux profiles on the fly
- [Bundles](/concepts/bundles) and [Fragments](/concepts/fragments) - Core concepts
- [Profiles](/concepts/profiles) - Save common combinations
- [Discovery](/guides/discovery) - Find community bundles
- [Sharing](/guides/sharing) - Publish your own bundles
