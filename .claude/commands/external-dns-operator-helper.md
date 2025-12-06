```yaml
name: external-dns-operator-helper
description: Run comprehensive PR review workflow for External DNS Operator changes with CI analysis
parameters:
  - name: pr_url
    description: GitHub PR URL to review (e.g., https://github.com/openshift/external-dns-operator/pull/123)
    required: true
```

I'll run a comprehensive automated review of the External DNS Operator PR. This includes checking out the PR, analyzing CI status, validating commits, checking Effective Go style, running local verification, and providing actionable feedback.

```bash
bash .claude/scripts/pr-review.sh "{{pr_url}}"
```

---

## What This Does

**Single command execution** that runs all checks without user interaction:

1. **PR Info** - Shows commits, files changed, PR size
2. **CI Status** - Displays Prow job results with clickable links (no deep log parsing)
3. **Commit Validation** - Checks JIRA-ID format
4. **Effective Go** - Validates receiver names, error strings, exported docs
5. **Make Targets** - Runs verify, test, build
6. **Specialized** - API/CRD sync, controller/test coverage, docs
7. **Summary** - Clear pass/fail report



## Requirements

- Git with `upstream` remote
- `jq` (optional - degrades gracefully without it)

## Usage

```bash
/external-dns-operator-helper https://github.com/openshift/external-dns-operator/pull/294
```