# shh

`shh` is a terminal UI for curating your SSH hosts. It keeps a small SQLite database of hosts you reach for most often, imports candidates from your shell history, and lets you connect with a couple of keystrokes.

## Features

- Fuzzy search across hostnames and comments.
- Keyboard-driven workflow (`/` to search, `a/e/d` to add/edit/delete, `PgUp/PgDn` for fast navigation).
- Tracks usage statistics (last used time, connect count).
- Imports hosts from existing shell history (`r`).
- Prints or executes the SSH command depending on flags.

## Install

```bash
go install github.com/Uri2001/shh/cmd/shh@latest
```

Or clone the repo and build locally:

```bash
git clone https://github.com/Uri2001/shh.git
cd shh
make build   # binary in ./bin/shh
```

## Usage

Launch the TUI:

```bash
shh          # after go install
# or
make run     # from the project folder
```

Keyboard shortcuts inside the UI:

| Key                  | Action                              |
|----------------------|-------------------------------------|
| `/`                  | Focus search input                  |
| `Esc`                | Clear search                        |
| `Enter`              | Connect (or exit when printing)     |
| `PgUp` / `PgDn`      | Page through the list               |
| `F4` / `F3` / `Del`  | Add / edit / delete host            |
| `F5`                 | Import from shell history           |
| `Ctrl+C`             | Quit                                |

Command-line flags:

```
-print    Print the selected host and exit
-cmd      Print the SSH command and exit
```

## Development

```bash
make fmt     # go fmt ./...
make test    # go test ./...
make vet     # go vet ./...
make tidy    # go mod tidy
```

The primary entry point lives at `cmd/shh/main.go`. Helper code for Windows/non-Windows console handling is colocated in the same directory.

## License

MIT. See [LICENSE](LICENSE).

## Contributing

Read [CONTRIBUTING.md](CONTRIBUTING.md) for details on reporting issues, proposing enhancements, and submitting pull requests.
