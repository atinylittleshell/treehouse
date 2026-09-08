# Contributing to Treehouse

Thanks for your interest in contributing!

## Getting Started

```sh
git clone https://github.com/kunchenguid/treehouse.git
cd treehouse
make build
make test
```

## Making Changes

1. Fork the repo and create a branch from `main`.
2. Make your changes.
3. Run `make lint` and `make test` to verify.
4. Open a pull request (see **Contribution Gate** below).

## Contribution Gate

PRs to `main` must be raised through [no-mistakes](https://github.com/kunchenguid/no-mistakes).
The required check `PR must be raised via no-mistakes` enforces this — a hand-opened PR
will stay red until the branch is pushed through the gate. The gate writes a pipeline
attestation whose `head_sha` matches the PR head; a hand-added marker is not enough.

### Setup (one-time)

```sh
# Install no-mistakes
curl -fsSL https://raw.githubusercontent.com/kunchenguid/no-mistakes/main/docs/install.sh | sh
no-mistakes doctor   # needs git + a supported agent runner + gh

# In your fork clone, keep origin on your fork and add the parent as upstream
git remote add upstream https://github.com/kunchenguid/treehouse.git

# Point the gate at your fork
no-mistakes init --fork-url https://github.com/<you>/treehouse.git
```

### Raising a PR

```sh
# On your feature branch, push through the gate (not git push origin)
git push no-mistakes

# Drive the pipeline to completion
no-mistakes                    # TUI, or:
no-mistakes axi run --intent "<what you set out to accomplish>"
# Loop on gate: with no-mistakes axi respond --action approve|fix|skip
# until outcome: checks-passed
```

The gate pushes your branch to your fork, opens (or updates) the PR against the parent
repo, and writes the attestation into the PR body. Once the pipeline hits
`checks-passed`, the `PR must be raised via no-mistakes` check goes green on its own.

### Workflow file changes

If your PR touches `.github/workflows/*.yml`, the push to your fork requires a git
credential with the `workflow` scope. GitHub rejects the push with
`refusing to allow an OAuth App to create or update workflow ... without workflow scope`
if the stored token lacks it. Fix it before pushing:

```sh
# Ensure your gh token has workflow + repo scope, then configure git to use it:
gh auth setup-git
```

### Tips

- The attestation's `head_sha` must match the current PR head. If you force-push,
  rebase, or the pipeline auto-fixes land new commits, re-run
  `no-mistakes axi run` (or `no-mistakes rerun`) to produce a fresh attestation —
  do not hand-edit the PR body.
- Let the pipeline make its own fix commits (review/test/lint auto-fixes land on
  the branch). Don't abort-and-restart to fix a finding yourself mid-run; respond
  at the gate instead.

See the [no-mistakes quick start](https://kunchenguid.github.io/no-mistakes/start-here/quick-start/)
for the full walkthrough.

## Guidelines

- Keep PRs focused — one feature or fix per PR.
- Follow existing code style (`gofmt` is enforced in CI).
- Add tests for new functionality when possible.

## Reporting Issues

Open a [GitHub issue](https://github.com/kunchenguid/treehouse/issues) with steps to reproduce. Include your OS, architecture, and treehouse version (`treehouse --version`).

## License

By contributing, you agree that your contributions will be licensed under the [MIT License](LICENSE).
