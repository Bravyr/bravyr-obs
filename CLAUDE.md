# CLAUDE.md

This file provides guidance to Claude Code when working with the bravyr-obs repository.

**Rules:**
- Never push directly to `main` -- always use PRs
- Feature branches are short-lived (merge within days, not weeks)
- Branch naming: `feature/`, `fix/`, `chore/`, `docs/`
- Delete feature branches after merge

## Workflow

### Planning
When planning any task, use these 4 agents to review requirements and design the implementation together:
- **product-owner** -- gap analysis between spec and frontend needs
- **software-architect** -- file structure, handler organization, patterns
- **security-specialist** -- auth, input validation, OWASP risks
- **dba** -- schema verification, query design, indexes

Always save the final plan as a markdown file in `docs/plans/` with the format `{issue_number}-{short-name}.md` (e.g., `035-temporal-worker-infrastructure.md`). This ensures plans are versioned in git and accessible across sessions.

### Changelog
**CHANGELOG.md must be updated with every commit.** Document all added, changed, fixed, or removed items under the appropriate version heading. Follow the [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) format.

### README
Update `README.md` if the changes affect project setup, features, structure, or developer-facing documentation.

### Pre-Commit Checks
Before every commit, run `make swagger`, `make lint`, and `make test` (or `go test ./...`) and fix any failures. Do not commit code that fails lint or tests.

### Authorship
Never mention Claude, AI, co-authored, or any AI tooling in code, comments, commit messages, or PR descriptions.

### Zero Technical Debt Policy
Fix ALL review findings before committing — never defer issues as "acceptable for V1." Every finding from every review agent must be addressed, no exceptions. If a fix is genuinely out of scope, create a GitHub issue for it immediately.

### Pre-Commit Review
After implementation and before committing, run these 5 review agents:
- **code-reviewer** -- code quality, patterns, style
- **pr-test-analyzer** -- test coverage and gaps
- **security-specialist** -- vulnerability scan
- **software-architect** -- architectural fitness
- **dba** -- query efficiency, index coverage

### Issue Management
When starting work on an issue, update it: assign to brunosansigolo, and link the feature branch and PR.

### Pull Request Standards
When creating a PR with `gh pr create`, always include:
- `--assignee brunosansigolo`
- `--label` with the relevant labels from the linked issue (e.g., `phase:4-background-jobs,type:feature,priority:high`)
- If the issue has labels, copy them to the PR. If no labels exist, use at minimum `type:feature`, `type:task`, or the appropriate type label.

### Deferred Work Policy
When planning or implementing a feature, any work that is explicitly deferred or skipped must have a GitHub issue created for it. This ensures deferred items are tracked and not forgotten. Use `gh issue create` with appropriate labels, and assign to brunosansigolo.