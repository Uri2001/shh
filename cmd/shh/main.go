package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	_ "modernc.org/sqlite"
)

const (
	appName       = "shh"
	dbFileName    = "hosts.db"
	importDoneKey = "import_done"
)

var version = "dev"

func dataDir() (string, error) {
	if runtime.GOOS == "linux" {
		if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
			return filepath.Join(xdg, appName), nil
		}
		h, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(h, ".local", "share", appName), nil
	}
	h, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(h, "."+appName), nil
}

// openDB opens (or creates) the application's SQLite database in the user's data directory.
func openDB() (*sql.DB, error) {
	d, err := dataDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(d, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(d, dbFileName)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if err := ensureSchema(db); err != nil {
		return nil, err
	}
	return db, nil
}

func ensureSchema(db *sql.DB) error {
	statements := []string{
		`PRAGMA journal_mode=WAL;`,
		`CREATE TABLE IF NOT EXISTS hosts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			host TEXT NOT NULL UNIQUE,
			comment TEXT,
			last_used_at TIMESTAMP NULL,
			use_count INTEGER NOT NULL DEFAULT 0
		);`,
		`CREATE TABLE IF NOT EXISTS meta (
			key TEXT PRIMARY KEY,
			value TEXT
		);`,
		`CREATE INDEX IF NOT EXISTS idx_hosts_last_used ON hosts(last_used_at DESC);`,
	}
	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

func execInUserShellLogin(host string) error {
	host, err := normalizeHost(host)
	if err != nil {
		return err
	}
	cleanupTerminal()
	if runtime.GOOS == "windows" {
		cmd := exec.Command("ssh", host)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return err
		}
		return nil
	}

	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash"
	}
	cmd := fmt.Sprintf("exec ssh %s", shellQuote(host))
	base := filepath.Base(shell)
	var argv []string
	switch base {
	case "bash", "zsh", "fish":
		argv = []string{shell, "-l", "-i", "-c", cmd}
	default:
		argv = []string{shell, "-i", "-c", cmd}
	}
	if err := syscall.Exec(shell, argv, os.Environ()); err != nil {
		return fmt.Errorf("exec shell: %w", err)
	}
	return nil
}

func cleanupTerminal() {
	restoreConsoleState()
	if !isTerminal(os.Stdout) {
		return
	}
	fmt.Fprint(os.Stdout, "\x1b[0m\x1b[?25h")
}

func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

type exitError struct {
	code int
}

func (e *exitError) Error() string {
	return fmt.Sprintf("exit status %d", e.code)
}

func (e *exitError) ExitCode() int {
	return e.code
}

func run() error {
	captureConsoleState()
	defer cleanupTerminal()

	var showVersion bool
	flag.BoolVar(&showVersion, "version", false, "print version and exit")
	flag.BoolVar(&showVersion, "v", false, "print version and exit")
	printHost := flag.Bool("print", false, "print selected host and exit")
	printCmd := flag.Bool("cmd", false, "print command 'ssh <host>' and exit")
	flag.Parse()

	if showVersion {
		fmt.Println(version)
		return nil
	}

	mode := runExecShell
	if *printHost {
		mode = runPrintHost
	}
	if *printCmd {
		mode = runPrintCmd
	}

	db, err := openDB()
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer db.Close()
	defer func() {
		_, _ = db.Exec("PRAGMA optimize")
	}()

	store := NewStore(db)
	ctx := context.Background()

	metaVal, ok, err := store.GetMeta(ctx, importDoneKey)
	if err != nil {
		return fmt.Errorf("get meta: %w", err)
	}
	if !ok || metaVal != "1" {
		imported, err := store.ImportFromHistory(ctx)
		if err != nil {
			return fmt.Errorf("import history: %w", err)
		}
		if imported > 0 {
			fmt.Fprintf(os.Stderr, "Imported from history: %d hosts\n", imported)
		}
		if err := store.SetMeta(ctx, importDoneKey, "1"); err != nil {
			return fmt.Errorf("set meta: %w", err)
		}
	}

	m, err := newModel(ctx, store, mode)
	if err != nil {
		return fmt.Errorf("init ui: %w", err)
	}
	opts := []tea.ProgramOption{}
	if runtime.GOOS != "windows" {
		opts = append(opts, tea.WithAltScreen())
	}
	prog := tea.NewProgram(m, opts...)
	res, err := prog.Run()
	if err != nil {
		return fmt.Errorf("run ui: %w", err)
	}
	final := res.(model).finalHost
	if final == "" {
		return nil
	}

	switch mode {
	case runExecShell:
		if err := execInUserShellLogin(final); err != nil {
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				return &exitError{code: exitErr.ExitCode()}
			}
			return err
		}
	case runPrintHost:
		fmt.Println(final)
	case runPrintCmd:
		fmt.Printf("ssh %s\n", final)
	}
	return nil
}

func main() {
	if err := run(); err != nil {
		var exitErr *exitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.ExitCode())
		}
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
