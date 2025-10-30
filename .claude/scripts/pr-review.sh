#!/bin/bash
set -euo pipefail

# ═══════════════════════════════════════════════════════════
# External DNS Operator PR Review - Simplified Single-Run
# ═══════════════════════════════════════════════════════════

REVIEW_START=$(date +%s)
PR_URL="$1"
PR_NUMBER=$(echo "$PR_URL" | grep -oE '[0-9]+$')

if [ -z "$PR_NUMBER" ]; then
    echo "❌ ERROR: Invalid PR URL"
    exit 1
fi

CURRENT_BRANCH=$(git branch --show-current)
REVIEW_BRANCH="pr-review-$PR_NUMBER"

# Cleanup on exit
cleanup() {
    if [ "$(git branch --show-current)" = "$REVIEW_BRANCH" ]; then
        git checkout "$CURRENT_BRANCH" 2>/dev/null || true
    fi
    git branch -D "$REVIEW_BRANCH" 2>/dev/null || true
}
trap cleanup EXIT

echo "📍 Current branch: $CURRENT_BRANCH"
echo "🔍 Reviewing PR #$PR_NUMBER"
echo ""

# Pre-flight checks
if ! git diff --quiet || ! git diff --cached --quiet; then
    echo "❌ ERROR: Uncommitted changes detected"
    git status --porcelain
    exit 1
fi

if ! git remote | grep -q "^upstream$"; then
    echo "❌ ERROR: 'upstream' remote not found"
    echo "   Add with: git remote add upstream https://github.com/openshift/external-dns-operator.git"
    exit 1
fi

echo "✅ Working directory clean"
echo "✅ Upstream remote found"
echo ""

# Fetch and checkout PR
echo "🔄 Fetching PR #$PR_NUMBER..."
git fetch upstream pull/$PR_NUMBER/head:$REVIEW_BRANCH 2>&1 | grep -v "^remote:" || true
git checkout "$REVIEW_BRANCH" >/dev/null 2>&1
echo ""

# ═══════════════════════════════════════════════════════════
# PR Information
# ═══════════════════════════════════════════════════════════

echo "═══════════════════════════════════════════════════════════"
echo "                    PR INFORMATION"
echo "═══════════════════════════════════════════════════════════"
echo ""

COMMIT_COUNT=$(git rev-list --count upstream/main..HEAD)
echo "📋 Commits: $COMMIT_COUNT"
git log upstream/main..HEAD --oneline
echo ""

CHANGED_FILES=$(git diff --name-status upstream/main...HEAD | wc -l)
LINES_CHANGED=$(git diff upstream/main...HEAD --shortstat | grep -oP '\d+(?= insertion)|\d+(?= deletion)' | paste -sd+ | bc 2>/dev/null || echo "0")

echo "📁 Files changed: $CHANGED_FILES"
echo "📏 Lines changed: $LINES_CHANGED"
[ "$LINES_CHANGED" -gt 500 ] && echo "   ⚠️  Large PR - consider splitting"
echo ""

# ═══════════════════════════════════════════════════════════
# CI Status (Simple - No Log Parsing)
# ═══════════════════════════════════════════════════════════

echo "═══════════════════════════════════════════════════════════"
echo "🤖 CI STATUS (Prow)"
echo "═══════════════════════════════════════════════════════════"
echo ""

# Get commit SHA and fetch CI status (single API call)
COMMIT_SHA=$(git rev-parse HEAD)
CI_DATA=$(curl -s -f "https://api.github.com/repos/openshift/external-dns-operator/statuses/$COMMIT_SHA" 2>/dev/null || echo "")

if [ -n "$CI_DATA" ] && command -v jq &>/dev/null; then
    # Parse with jq if available
    echo "$CI_DATA" | jq -r '.[] | select(.context | startswith("ci/prow/") or startswith("ci/")) |
        "\(.state)|\(.context)|\(.target_url // "no-link")"' | sort -u | while IFS='|' read -r state context url; do
        case "$state" in
            success) echo "✅ $context" ;;
            failure|error)
                echo "❌ $context"
                [ "$url" != "no-link" ] && echo "   🔗 $url"
                ;;
            pending) echo "⏳ $context" ;;
            *) echo "❓ $context - $state" ;;
        esac
    done

    # Summary
    PASSED=$(echo "$CI_DATA" | jq -r '.[] | select(.state == "success") | .context' | grep -c "ci/" || echo "0")
    FAILED=$(echo "$CI_DATA" | jq -r '.[] | select(.state == "failure" or .state == "error") | .context' | grep -c "ci/" || echo "0")
    echo ""
    if [ "$FAILED" -eq 0 ]; then
        echo "✅ CI Summary: $PASSED jobs passed"
    else
        echo "⚠️  CI Summary: $PASSED passed, $FAILED failed - check links above"
    fi
elif [ -n "$CI_DATA" ]; then
    # Fallback without jq - basic parsing
    echo "$CI_DATA" | grep -o '"context":"ci/[^"]*"' | cut -d'"' -f4 | while read -r context; do
        if echo "$CI_DATA" | grep -q "\"context\":\"$context\".*\"state\":\"success\""; then
            echo "✅ $context"
        elif echo "$CI_DATA" | grep -q "\"context\":\"$context\".*\"state\":\"failure\""; then
            echo "❌ $context"
        else
            echo "⏳ $context"
        fi
    done
    echo ""
    echo "ℹ️  Install jq for full CI details: sudo dnf install jq"
else
    echo "ℹ️  Could not fetch CI status (check network or wait for CI to start)"
fi
echo ""

# ═══════════════════════════════════════════════════════════
# Step 1: Commit Message Validation
# ═══════════════════════════════════════════════════════════

echo "═══════════════════════════════════════════════════════════"
echo "⏳ Step 1/5: Commit message validation"
echo "═══════════════════════════════════════════════════════════"
echo ""

COMMIT_FAILED=false
while IFS= read -r commit; do
    hash=$(echo "$commit" | awk '{print $1}')
    msg=$(echo "$commit" | cut -d' ' -f2-)

    if ! echo "$msg" | grep -qE "^[A-Z]+-[0-9]+: "; then
        echo "   ❌ $hash: $msg"
        echo "      ^ Missing JIRA-ID (e.g., NE-2076: Description)"
        COMMIT_FAILED=true
    else
        echo "   ✅ $hash: $msg"
    fi
done < <(git log upstream/main..HEAD --oneline)

echo ""
[ "$COMMIT_FAILED" = true ] && echo "❌ FAILED" || echo "✅ PASSED"
echo ""

# ═══════════════════════════════════════════════════════════
# Step 2: Effective Go Style Checks
# ═══════════════════════════════════════════════════════════

echo "═══════════════════════════════════════════════════════════"
echo "⏳ Step 2/5: Effective Go style (https://go.dev/doc/effective_go)"
echo "═══════════════════════════════════════════════════════════"
echo ""

STYLE_WARNINGS=0

# Check 1: Receiver names
echo "🔎 Receiver names:"
RECEIVERS=$(git diff upstream/main...HEAD | grep -E '^\+.*func \([a-zA-Z_]+ \*?[A-Z]' | grep -oP 'func \(\K[a-zA-Z_]+' | sort -u || true)
if [ -n "$RECEIVERS" ]; then
    if echo "$RECEIVERS" | grep -qE '^(this|self|me)$'; then
        echo "   ❌ Avoid: this, self, me"
        STYLE_WARNINGS=$((STYLE_WARNINGS + 1))
    else
        echo "   ✅ Good: $(echo "$RECEIVERS" | tr '\n' ', ' | sed 's/, $//')"
    fi
else
    echo "   ℹ️  No new methods"
fi
echo ""

# Check 2: Error strings
echo "🔎 Error strings (lowercase, no punctuation):"
ERROR_ISSUES=$(git diff upstream/main...HEAD | \
    grep -E '^\+.*(errors\.New|fmt\.Errorf)\("' | \
    grep -v '//' | \
    grep -oP '(errors\.New|fmt\.Errorf)\("\K[^"]+' | \
    grep -E '^[A-Z][a-z]|\.$' || true)

if [ -n "$ERROR_ISSUES" ]; then
    echo "   ❌ Found issues:"
    echo "$ERROR_ISSUES" | head -3 | while read -r err; do
        echo "      \"$err\""
    done
    STYLE_WARNINGS=$((STYLE_WARNINGS + 1))
else
    echo "   ✅ Properly formatted"
fi
echo ""

# Check 3: Exported function docs
echo "🔎 Exported function docs:"
UNDOC=$(git diff upstream/main...HEAD | \
    grep -B1 -E '^\+func [A-Z]' | \
    grep -v -E '^\+//|^--' | \
    grep -E '^\+func [A-Z]' | \
    grep -oP 'func \K[A-Z][a-zA-Z0-9]+' || true)

if [ -n "$UNDOC" ]; then
    echo "   ⚠️  May need docs:"
    echo "$UNDOC" | head -3 | while read -r fn; do
        echo "      • $fn"
    done
    STYLE_WARNINGS=$((STYLE_WARNINGS + 1))
else
    echo "   ✅ Documented"
fi
echo ""

[ $STYLE_WARNINGS -eq 0 ] && echo "✅ PASSED" || echo "⚠️  $STYLE_WARNINGS warning(s)"
echo ""

# ═══════════════════════════════════════════════════════════
# Step 3: Verification
# ═══════════════════════════════════════════════════════════

echo "═══════════════════════════════════════════════════════════"
echo "⏳ Step 3/5: Verification (make verify)"
echo "═══════════════════════════════════════════════════════════"
echo ""

VERIFY_FAILED=false
if make verify 2>&1 | tail -30; then
    echo ""
    echo "✅ PASSED"
else
    echo ""
    echo "❌ FAILED - Run 'make verify' locally"
    VERIFY_FAILED=true
fi
echo ""

# ═══════════════════════════════════════════════════════════
# Step 4: Unit Tests
# ═══════════════════════════════════════════════════════════

echo "═══════════════════════════════════════════════════════════"
echo "⏳ Step 4/5: Unit tests (make test)"
echo "═══════════════════════════════════════════════════════════"
echo ""

TEST_FAILED=false
if timeout 600 make test 2>&1 | tail -30; then
    echo ""
    echo "✅ PASSED"
    [ -f coverage.out ] && echo "" && echo "📊 Coverage:" && go tool cover -func=coverage.out | tail -3
else
    echo ""
    echo "❌ FAILED - Run 'make test' locally"
    TEST_FAILED=true
fi
echo ""

# ═══════════════════════════════════════════════════════════
# Step 5: Build
# ═══════════════════════════════════════════════════════════

echo "═══════════════════════════════════════════════════════════"
echo "⏳ Step 5/5: Build (make build)"
echo "═══════════════════════════════════════════════════════════"
echo ""

BUILD_FAILED=false
if make build 2>&1 | tail -15; then
    echo ""
    echo "✅ PASSED"
    [ -f bin/external-dns-operator ] && ls -lh bin/external-dns-operator
else
    echo ""
    echo "❌ FAILED - Check compilation errors"
    BUILD_FAILED=true
fi
echo ""

# ═══════════════════════════════════════════════════════════
# Specialized Checks
# ═══════════════════════════════════════════════════════════

echo "═══════════════════════════════════════════════════════════"
echo "🔍 Specialized Checks"
echo "═══════════════════════════════════════════════════════════"
echo ""

SPEC_WARNINGS=0

# API changes → CRDs
if git diff upstream/main...HEAD --name-only | grep -q "api/.*types\.go"; then
    echo "🔎 API changes detected"
    if ! git diff upstream/main...HEAD --name-only | grep -q "config/crd/bases/"; then
        echo "   ❌ CRDs not updated - Run 'make manifests'"
        SPEC_WARNINGS=$((SPEC_WARNINGS + 1))
    else
        echo "   ✅ CRDs updated"
    fi
    echo ""
fi

# Controller → Tests
CTRL=$(git diff upstream/main...HEAD --name-only | grep "pkg/operator/controller/.*\.go" | grep -v "_test\.go" || true)
if [ -n "$CTRL" ]; then
    echo "🔎 Controller changes"
    MISSING=false
    while read -r file; do
        test_file="${file%.go}_test.go"
        if [ ! -f "$test_file" ] && ! git diff upstream/main...HEAD --name-only | grep -q "$test_file"; then
            echo "   ⚠️  $file - no test update"
            MISSING=true
            SPEC_WARNINGS=$((SPEC_WARNINGS + 1))
        fi
    done <<< "$CTRL"
    [ "$MISSING" = false ] && echo "   ✅ Tests updated"
    echo ""
fi

# User-facing → Docs
if git diff upstream/main...HEAD --name-only | grep -qE "(api/.*types\.go|config/samples/)"; then
    if ! git diff upstream/main...HEAD --name-only | grep -qE "(docs/|README\.md)"; then
        echo "🔎 User-facing changes - consider updating docs"
        SPEC_WARNINGS=$((SPEC_WARNINGS + 1))
        echo ""
    fi
fi

[ $SPEC_WARNINGS -eq 0 ] && echo "✅ No warnings" && echo ""

# ═══════════════════════════════════════════════════════════
# Summary
# ═══════════════════════════════════════════════════════════

echo "═══════════════════════════════════════════════════════════"
echo "                  SUMMARY"
echo "═══════════════════════════════════════════════════════════"
echo ""

ISSUES=0
echo "✓ Commit messages:    $([ "$COMMIT_FAILED" = true ] && echo "❌ FAILED" && ISSUES=$((ISSUES+1)) || echo "✅ PASSED")"
echo "✓ Effective Go:       $([ $STYLE_WARNINGS -gt 0 ] && echo "⚠️  $STYLE_WARNINGS warning(s)" || echo "✅ PASSED")"
echo "✓ Verification:       $([ "$VERIFY_FAILED" = true ] && echo "❌ FAILED" && ISSUES=$((ISSUES+1)) || echo "✅ PASSED")"
echo "✓ Tests:              $([ "$TEST_FAILED" = true ] && echo "❌ FAILED" && ISSUES=$((ISSUES+1)) || echo "✅ PASSED")"
echo "✓ Build:              $([ "$BUILD_FAILED" = true ] && echo "❌ FAILED" && ISSUES=$((ISSUES+1)) || echo "✅ PASSED")"
echo "✓ Specialized checks: $([ $SPEC_WARNINGS -gt 0 ] && echo "⚠️  $SPEC_WARNINGS warning(s)" || echo "✅ PASSED")"

echo ""
echo "═══════════════════════════════════════════════════════════"
echo ""

if [ $ISSUES -eq 0 ]; then
    echo "✅ OVERALL: PR #$PR_NUMBER is ready for merge!"
    [ $((STYLE_WARNINGS + SPEC_WARNINGS)) -gt 0 ] && echo "   ($((STYLE_WARNINGS + SPEC_WARNINGS)) warning(s) - consider addressing)"
else
    echo "❌ OVERALL: PR #$PR_NUMBER requires $ISSUES fix(es)"
fi

REVIEW_END=$(date +%s)
DURATION=$((REVIEW_END - REVIEW_START))
echo ""
echo "⏱️  Review took ${DURATION}s"
echo ""
echo "✅ Returned to: $CURRENT_BRANCH"
