# Repository Guidelines

## Project Structure & Module Organization
This repository is currently design-first and documentation-heavy. Keep top-level intent documents in the root:

- `README.md`: brief repository overview
- `docs/agents/`: repository-specific agent and workflow instructions

When implementation starts, place runtime code under dedicated source directories instead of expanding the root further. Keep new contributor-facing docs in `docs/`.

## Build, Test, and Development Commands
There is no project-local build or test pipeline yet. Until the controller scaffold is added, use lightweight validation commands before opening a PR:

- `git diff --check`: catch trailing spaces and malformed patches
- `git status --short`: confirm intended file scope
- `git log --oneline -5`: review recent commit style

If you introduce a real build or test command later, document it in `README.md` and update this file in the same change.

## Coding Style & Naming Conventions
Follow the repository's existing style:

- Write documentation in concise Japanese unless a file already uses English
- Use Markdown ATX headings (`#`, `##`, `###`) and short sections
- Prefer descriptive filenames such as `SPEC.md` or `commit-security-reviewer.md`
- Keep one topic per document; split large discussions into `docs/` rather than growing a single file indefinitely

Use examples and bullet lists when they improve scanability, but keep prose direct.

## Testing Guidelines
Because no automated tests exist yet, every change should include a manual review pass:

- verify links, headings, and code fences
- confirm terminology stays consistent across `README.md`, `SPEC.md`, and `PLAN.md`
- for process docs, check that commands and paths match the current repository layout

When code is added, place tests next to the relevant package or under a dedicated `tests/` directory and document the exact test command here.

## Commit & Pull Request Guidelines
Use Conventional Commits. This repository already documents that convention in `docs/agents/conventional-commit-writer.md`. Typical examples:

- `docs: clarify CRD status fields`
- `docs(agents): add commit review guidance`

Keep commit subjects in plain English, imperative, and specific. Pull requests should include a short Japanese summary, affected files, and any open questions. For GitHub operations, use `gh` rather than the web UI. If a change is large or restructures the repository, confirm the plan with the maintainer before editing.
