# shh

`shh` is a terminal UI for curating your SSH hosts. It keeps a small SQLite database of hosts you reach for most often, imports candidates from your shell history, and lets you connect with a couple of keystrokes.

## Features

- Fuzzy search across hostnames and comments.
- Keyboard-driven workflow (`/` to search, `Ctrl+A/E/D` to add/edit/delete, `Ctrl+R` to import, `PgUp/PgDn` for fast navigation).
- Tracks usage statistics (last used time, connect count).
- Imports hosts from existing shell history (`Ctrl+R`).
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
| `Ctrl+A` / `Ctrl+E` / `Ctrl+D` (Alt+N/E/D) | Add / edit / delete host |
| `Ctrl+R` (Alt+R)        | Import from shell history           |
| `Ctrl+C` / `q`       | Quit                                |

Command-line flags:

```
-print    Print the selected host and exit
-cmd      Print the SSH command and exit
```

## Data storage

`shh` keeps a lightweight SQLite database (via the pure-Go `modernc.org/sqlite` driver) with two tables:

- `hosts`: the curated list (`host`, optional `comment`, `last_used_at`, and `use_count`).
- `meta`: internal settings such as whether shell history import has already run.

The database lives in the per-user data directory:

- Linux: `$XDG_DATA_HOME/shh/hosts.db` (or `~/.local/share/shh/hosts.db` as a fallback).
- macOS / Windows: `~/.shh/hosts.db`.

WAL mode is enabled for reliability, and the app executes `PRAGMA optimize` on exit. Hostnames are validated before being written, so malicious entries such as `localhost; rm -rf /` are rejected. If you sync the database between machines, remember it contains plaintext hostnames and comments—treat it like any other local SSH configuration file.

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

