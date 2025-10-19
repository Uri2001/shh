# Contributing to shh

Thanks for your interest in improving `shh`! Please take a moment to review the guidelines below before opening an issue or submitting a pull request.

## Reporting Bugs

1. Search existing issues to avoid duplicates.
2. Include steps to reproduce, expected behaviour, and actual behaviour.
3. Share your OS, Go version (`go version`), and terminal emulator.

## Suggesting Enhancements

1. Check the issue tracker to see if the idea already exists.
2. Describe the problem your proposal solves and any alternatives you considered.
3. If possible, outline a rough implementation approach.

## Development Workflow

1. Fork the repository and create a feature branch (`git checkout -b feat/my-change`).
2. Keep the codebase formatted and vetted:
   ```bash
   make fmt
   make vet
   make test
   ```
3. Ensure `make build` succeeds and that the TUI still launches (`make run`).
4. Squash or rebase before opening the PR to keep history tidy.
5. Submit the PR with a clear description, screenshots/gifs if the UI changed, and reference related issues.

## Coding Standards

- Follow idiomatic Go style (`go fmt`).
- Keep user-facing strings in English unless behind localisation logic.
- Tests are encouraged for non-trivial changes; table-driven tests preferred.
- Avoid committing binaries or local databases (`bin/`, `*.db` are gitignored).

## Code of Conduct

Be respectful and constructive. Harassment of any kind will not be tolerated. Please report concerns privately to the maintainer.

Happy hacking!
