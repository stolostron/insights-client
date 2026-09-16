# insights-client

## Overview

`insights-client` is a Go service that retrieves health-check insights for managed OpenShift clusters and creates a `PolicyReport` custom resource for each cluster containing the insight violations.

The service starts in `main.go`. Configuration and Kubernetes clients are in `pkg/config`, insight retrieval is in `pkg/retriever`, report processing is in `pkg/processor`, cluster monitoring is in `pkg/monitor`, and shared types and utilities are in `pkg/types` and `pkg/utils`. The end-to-end test fixtures and Helm chart are under `test-data/e2e`.

## Development Commands

| Command | Purpose |
|---|---|
| `make deps` | Tidy Go module dependencies |
| `make build` | Build `output/insights-client` |
| `make build-linux` | Build for Linux |
| `make test` | Run all Go tests with verbose output and write `cover.out` |
| `make lint` | Install and run `golangci-lint` v2.9.0 with a three-minute timeout |
| `make run` | Run the service with `go run main.go` |
| `make e2e-tests` | Run the local end-to-end test script |
| `make test-e2e-prow` | Run the Prow end-to-end test script |
| `make clean` | Remove Go build artifacts and coverage files |

Prow equivalents are `make unit-test` and `make test-e2e` from `Makefile.prow`. The build harness bootstrap in `Makefile` may download a remote template unless `USE_VENDORIZED_BUILD_HARNESS` is set.

For local development, run `sh setup.sh` to generate the self-signed certificate, then authenticate with `oc login` or set `KUBECONFIG` before `make run`.

## Configuration

The service reads `HTTP_TIMEOUT`, `CCX_SERVER`, `CCX_TOKEN`, `POLL_INTERVAL`, `REQUEST_INTERVAL`, and `CACERT`. `CCX_TOKEN` can instead be obtained from the `openshift-config` secret when unset. See `README.md` for defaults and intended development-only settings.

## Repository Conventions

- Keep Go code formatted with `gofmt` and add or update tests for behavior changes.
- Do not commit generated certificates, coverage output, or build output; these paths are ignored.
- Include the related Jira issue in pull requests using `.github/pull_request_template.md`.
- The project uses DCO sign-offs for commits.

## Personal configuration

Read personal config at the start of any task that needs an assignee, email, or project key.
Canonical path: `~/.config/user.local.md` (tool-agnostic, global).
If the file does not exist, fall back to agent memory (`user-config`), then placeholders.
Run `make personalize` to generate or update the file (if this repo uses Fleet Engineering tooling).

## Tool Integrations

This repository does not define a `make personalize` target. The `gh` and `jira` CLIs are not available in this environment; use the configured GitHub MCP and Jira MCP integrations when available. GitHub access may use `GH_TOKEN` or an organization-specific `GH_TOKEN_<ORG>` convention where applicable.

## Fleet Engineering Skills

Fetch and apply the relevant skill when the task matches its domain.

| Skill | When to use |
|---|---|
| [bug-specialist](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/bug-specialist/SKILL.md) | Bug triage, reproduction steps, fix planning |
| [epic-specialist](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/epic-specialist/SKILL.md) | Multi-sprint epics with outcomes |
| [feature-specialist](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/feature-specialist/SKILL.md) | Large customer-facing capabilities |
| [initiative-specialist](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/initiative-specialist/SKILL.md) | Multi-team strategic programs |
| [jira-create](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/jira-create/SKILL.md) | Interactive issue creation with specialist delegation |
| [jira-qe-readiness](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/jira-qe-readiness/SKILL.md) | Check whether a Jira ticket has enough information for QE to verify |
| [jira-report](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/jira-report/SKILL.md) | Jira portfolio reports and issue quality reviews |
| [jira-specialist](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/jira-specialist/SKILL.md) | General triage, search, linking, and transitions |
| [jira-type-audit](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/jira-type-audit/SKILL.md) | Audit and correct Jira issue types across the hierarchy |
| [outcome-specialist](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/outcome-specialist/SKILL.md) | Strategic outcomes tied to OKRs |
| [release-dod](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/release-dod/SKILL.md) | Release Definition of Done checklists |
| [risk-report](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/risk-report/SKILL.md) | Automated risk signal detection and status report drafting |
| [risk-specialist](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/risk-specialist/SKILL.md) | Risk registers, mitigation planning, and hazard tracking |
| [spike-specialist](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/spike-specialist/SKILL.md) | Time-boxed research and proofs of concept |
| [story-specialist](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/story-specialist/SKILL.md) | User stories with acceptance criteria |
| [supportex-review](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/supportex-review/SKILL.md) | Review and approve SUPPORTEX support exceptions |
| [task-specialist](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/task-specialist/SKILL.md) | Internal technical tasks |
| [ticket-specialist](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/ticket-specialist/SKILL.md) | Stakeholder request intake, triage, and conversion |
| [backlog-grooming](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/backlog-grooming/SKILL.md) | Scan a Jira project or component and produce a grooming agenda |
| [breaking-changes](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/breaking-changes/SKILL.md) | Detect breaking API, database, config, behavior, and integration changes |
| [ci-triage](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/ci-triage/SKILL.md) | Diagnose failing CI checks on a pull request |
| [coderabbit-sync](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/coderabbit-sync/SKILL.md) | Maintain the Fleet reference `.coderabbit.yaml` |
| [cve-sustaining-handoff](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/cve-sustaining-handoff/SKILL.md) | Resolve CVE trackers and hand off z-stream work to sustaining |
| [vulnerability-slack-report](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/vulnerability-slack-report/SKILL.md) | Post weekly summaries of overdue vulnerability issues |
| [diagnosing-bugs](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/diagnosing-bugs/SKILL.md) | Debug unclear failures with reproduce-and-minimize workflow |
| [finish-work](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/finish-work/SKILL.md) | Commit, push, open a pull request, and update Jira |
| [github-org-access](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/github-org-access/SKILL.md) | Modify GitHub organization access through Peribolos YAML |
| [init-context-docs](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/init-context-docs/SKILL.md) | Assess and bootstrap repository AI-readiness documentation |
| [opencode-setup](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/opencode-setup/SKILL.md) | Install and configure OpenCode |
| [org-repo-audit](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/org-repo-audit/SKILL.md) | Audit GitHub organization repositories for SDLC readiness |
| [pr-fix](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/pr-fix/SKILL.md) | Fix merge conflicts, CI failures, and review comments |
| [pr-hygiene](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/pr-hygiene/SKILL.md) | Manage stale open pull requests across workspace repositories |
| [pr-review](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/pr-review/SKILL.md) | Review GitHub pull requests with isolation and inline comments |
| [pr-review-detailed](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/pr-review-detailed/SKILL.md) | Perform layered checklist analysis of a branch or pull request |
| [pr-review-fix](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/pr-review-fix/SKILL.md) | Iteratively review and fix local changes before commit |
| [release-notes](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/release-notes/SKILL.md) | Generate categorized release notes between git references |
| [renovate-prs](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/renovate-prs/SKILL.md) | Manage Renovate and MintMaker dependency pull requests |
| [repo-content-audit](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/repo-content-audit/SKILL.md) | Scan for unlinked or orphaned repository content |
| [repo-setup](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/repo-setup/SKILL.md) | Onboard a repository to the Fleet Engineering Agentic SDLC |
| [rhacm-addon-wizard](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/rhacm-addon-wizard/SKILL.md) | Guide RHACM add-on development and scaffold generation |
| [scored-code-review](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/scored-code-review/SKILL.md) | Deprecated; use `pr-review-detailed` instead |
| [session-summary](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/session-summary/SKILL.md) | Correlate session history with Jira and GitHub for status reporting |
| [start-work](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/start-work/SKILL.md) | Create a Jira sub-task |
| [test-coverage-gap](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/test-coverage-gap/SKILL.md) | Analyze coverage gaps and suggest risk-prioritized tests |
| [f2f-daily-summary](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/f2f-daily-summary/SKILL.md) | Capture daily F2F meeting notes as Jira sub-tasks |
| [f2f-epic-specialist](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/f2f-epic-specialist/SKILL.md) | Create and manage F2F meeting epics |
| [presentation-task](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/presentation-task/SKILL.md) | Log delivered presentations as closed Jira sub-tasks |
| [scrum-status](https://raw.githubusercontent.com/OpenShift-Fleet/agentic-sdlc/main/skills/scrum-status/SKILL.md) | Capture scrum bullets and generate weekly team reports |
