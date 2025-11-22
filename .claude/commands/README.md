# External DNS Operator - Claude Code Commands

This directory contains custom Claude Code slash commands to help with development and PR review workflows.

## Available Commands

### `/external-dns-operator-helper`

Automated PR review workflow that runs comprehensive quality checks on Pull Requests.

---

## Prerequisites

You need **Claude Code** installed and configured. See [Claude Code Documentation](https://docs.google.com/document/d/1eNARy9CI28o09E7Foq01e5WD5MvEj3LSBnXqFcprxjo/edit?tab=t.0#heading=h.8ldy5by9bpo8) for installation instructions.

---

## How to Use the `/external-dns-operator-helper` Command

### Step 1: Open Claude Code

Launch Claude Code in your terminal or IDE.

### Step 2: Navigate to Repository

Ensure you're in the external-dns-operator repository directory:

```bash
cd /path/to/external-dns-operator
```

### Step 3: Run the Command

In Claude Code, use the slash command with a PR URL:

```
/external-dns-operator-helper https://github.com/openshift/external-dns-operator/pull/123
```

**Replace `123` with the actual PR number you want to review.**

### Step 4: Review the Results

Claude Code will automatically:
1. ✅ Check for uncommitted changes and save your current branch
2. ✅ Checkout the PR
3. ✅ Run `make verify` (lint, format, deps, generated code, OLM)
4. ✅ Run `make test` (unit tests with coverage)
5. ✅ Run `make build` (compilation check)
6. ✅ Perform specialized checks (API changes, tests, docs)
7. ✅ Generate comprehensive report
8. ✅ Return to your original branch

---

## Common Use Cases

### Use Case 1: Pre-Submission PR Check

**Before creating or updating your PR**, validate your changes:

```bash
# 1. Push your changes to your fork
git push origin your-branch-name

# 2. Create the PR (via GitHub UI or gh CLI)
# Get the PR URL, then run:
/external-dns-operator-helper https://github.com/openshift/external-dns-operator/pull/YOUR-PR-NUMBER
```

### Use Case 2: Review Someone Else's PR

**Before manually reviewing**, run automated checks:

```bash
# Someone asks you to review PR #296
/external-dns-operator-helper https://github.com/openshift/external-dns-operator/pull/296
```

If automated checks pass, you can focus on code logic and architecture review.

### Use Case 3: Debugging CI Failures

**When CI fails on your PR**, reproduce locally:

```bash
# Your PR #301 failed CI, check it locally:
/external-dns-operator-helper https://github.com/openshift/external-dns-operator/pull/301
```

The command will show the same errors CI would show, with detailed explanations.

### Use Case 4: Learning Best Practices

**New to the project?** Use it to learn what checks are required:

```bash
# Review a recent merged PR to see what passed:
/external-dns-operator-helper https://github.com/openshift/external-dns-operator/pull/296
```

---

## What the Command Checks

### 1. Linting (`make lint`)
- errcheck, gofmt, goimports, gosimple, govet
- ineffassign, misspell, staticcheck, typecheck, unused
- Configuration: `.golangci.yaml`

### 2. Formatting (`hack/verify-gofmt.sh`)
- All `.go` files must be `gofmt -s` compliant

### 3. Dependencies (`hack/verify-deps.sh`)
- `go.mod` and `go.sum` are in sync
- `vendor/` directory is up-to-date

### 4. Generated Code (`hack/verify-generated.sh`)
- DeepCopy methods are current (`make generate`)
- CRDs, RBAC, webhooks are current (`make manifests`)

### 5. OLM Bundle (`hack/verify-olm.sh`)
- Bundle manifests are up-to-date (`make bundle`)
- Catalog is current (`make catalog`)

### 6. Unit Tests (`make test`)
- All unit tests pass
- No race conditions (runs with `-race`)
- Coverage report generated

### 7. Build Validation (`make build`)
- Operator binary compiles successfully

### 8. Specialized Checks
- API changes → CRD updates
- Controller changes → Test updates
- User-facing changes → Documentation updates
- Kubebuilder markers → Proper documentation

---



## Additional Resources

- **AGENTS.md**: Comprehensive repository development guide
- **CLAUDE.md**: Symlink to AGENTS.md
- **Makefile**: Build targets and CI automation
- **docs/**: External DNS Operator documentation

---