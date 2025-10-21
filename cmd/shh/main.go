// =========================
// main.go
// =========================
package main

import (
	"bufio"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"time"

	table "github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sahilm/fuzzy"
	_ "modernc.org/sqlite"
)

const (
	appName       = "shh" // short command name
	dbFileName    = "hosts.db"
	importDoneKey = "import_done"
)

var version = "dev"

// ------------------------- paths -------------------------
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

// -------------------------- DB ---------------------------

type Host struct {
	ID         int64
	Host       string
	Comment    string
	LastUsedAt sql.NullTime
	UseCount   int
}

func openDB() (*sql.DB, error) {
	d, err := dataDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(d, 0o755); err != nil {
		return nil, err
	}
	p := filepath.Join(d, dbFileName)
	db, err := sql.Open("sqlite", p)
	if err != nil {
		return nil, err
	}
	if err := ensureSchema(db); err != nil {
		return nil, err
	}
	return db, nil
}

func ensureSchema(db *sql.DB) error {
	stmts := []string{
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
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return err
		}
	}
	return nil
}

func getMeta(db *sql.DB, key string) (string, bool, error) {
	var v string
	err := db.QueryRow(`SELECT value FROM meta WHERE key=?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	return v, err == nil, err
}

func setMeta(db *sql.DB, key, value string) error {
	_, err := db.Exec(`INSERT INTO meta(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}

func listHosts(db *sql.DB) ([]Host, error) {
	rows, err := db.Query(`SELECT id,host,comment,last_used_at,use_count FROM hosts ORDER BY 
		CASE WHEN last_used_at IS NULL THEN 1 ELSE 0 END, last_used_at DESC, host ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var r []Host
	for rows.Next() {
		var h Host
		var ts sql.NullTime
		if err := rows.Scan(&h.ID, &h.Host, &h.Comment, &ts, &h.UseCount); err != nil {
			return nil, err
		}
		h.LastUsedAt = ts
		r = append(r, h)
	}
	return r, rows.Err()
}

func addHost(db *sql.DB, host, comment string) error {
	if strings.TrimSpace(host) == "" {
		return fmt.Errorf("empty host")
	}
	_, err := db.Exec(`INSERT INTO hosts(host,comment) VALUES(?,?)`, strings.TrimSpace(host), strings.TrimSpace(comment))
	return err
}

func updateHost(db *sql.DB, id int64, host, comment string) error {
	_, err := db.Exec(`UPDATE hosts SET host=?, comment=? WHERE id=?`, strings.TrimSpace(host), strings.TrimSpace(comment), id)
	return err
}

func deleteHost(db *sql.DB, id int64) error {
	_, err := db.Exec(`DELETE FROM hosts WHERE id=?`, id)
	return err
}
func markUsed(db *sql.DB, id int64) error {
	_, err := db.Exec(`UPDATE hosts SET use_count=use_count+1, last_used_at=? WHERE id=?`, time.Now().UTC(), id)
	return err
}

// --------------------- history import --------------------

var sshCmdRegex = regexp.MustCompile(`(?i)(?:^|\s)ssh\s+((?:-\S+\s+\S+\s*)*)((?:[a-z0-9._-]+@)?(?:\[[0-9a-fA-F:]+\]|[a-z0-9._-]+))`)
var ansiRegexp = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)

func possibleHistoryFiles() []string {
	h, _ := os.UserHomeDir()
	return []string{filepath.Join(h, ".bash_history"), filepath.Join(h, ".zsh_history"), filepath.Join(h, ".local", "share", "fish", "fish_history")}
}

func parseHistoryLine(line string) (string, bool) {
	if strings.HasPrefix(line, ": ") {
		if i := strings.Index(line, ";"); i >= 0 {
			line = line[i+1:]
		}
	}
	m := sshCmdRegex.FindStringSubmatch(line)
	if m == nil {
		return "", false
	}
	h := m[2]
	if at := strings.LastIndex(h, "@"); at >= 0 {
		h = h[at+1:]
	}
	h = strings.Trim(h, "[]")
	if h == "" || strings.Contains(h, " ") || strings.HasPrefix(h, "-") {
		return "", false
	}
	return h, true
}

func importFromHistory(db *sql.DB) (int, error) {
	n := 0
	seen := map[string]struct{}{}
	for _, p := range possibleHistoryFiles() {
		f, err := os.Open(p)
		if err != nil {
			continue
		}
		s := bufio.NewScanner(f)
		for s.Scan() {
			if host, ok := parseHistoryLine(s.Text()); ok {
				if _, dup := seen[host]; dup {
					continue
				}
				seen[host] = struct{}{}
				_ = addHost(db, host, "imported from history")
				n++
			}
		}
		f.Close()
	}
	return n, nil
}

// --------------------------- TUI -------------------------

type mode int

const (
	modeList mode = iota
	modeAdd
	modeEdit
	modeConfirmDelete
)

type runMode int

const (
	runExecShell runMode = iota // on Enter: exec via shell (-l -i -c)
	runPrintHost                // on Enter: print host and exit
	runPrintCmd                 // on Enter: print 'ssh <host>' and exit
)

type model struct {
	db         *sql.DB
	mode       mode
	rmode      runMode
	table      table.Model
	search     textinput.Model
	inputs     []textinput.Model
	status     string
	allHosts   []Host
	filteredIx []int
	pageSize   int
	finalHost  string // selected host for exit
}

var (
	baseStyle   = lipgloss.NewStyle().Padding(0, 1).Foreground(lipgloss.Color("7"))
	headerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("10"))
	statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
)

func newModel(db *sql.DB, r runMode) (model, error) {
	m := model{db: db, rmode: r}
	m.search = textinput.New()
	m.search.Placeholder = "search (host/comment), / to focus, Esc to clear"
	m.search.Prompt = "/ "
	m.search.CharLimit = 256
	m.search.Focus()
	m.search.Width = 60
	m.search.PromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
	m.search.TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	cols := []table.Column{{Title: "Host", Width: 36}, {Title: "Comment", Width: 60}, {Title: "Last Used", Width: 19}, {Title: "#", Width: 4}}
	m.table = table.New(table.WithColumns(cols), table.WithHeight(15))
	m.pageSize = m.table.Height()
	if m.pageSize <= 0 {
		m.pageSize = 1
	}
	padding := lipgloss.NewStyle().Padding(0, 1)
	ts := table.DefaultStyles()
	ts.Header = padding.Copy().Bold(true).Foreground(lipgloss.Color("10"))
	ts.Cell = padding.Copy()
	ts.Selected = lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Bold(true)
	m.table.SetStyles(ts)
	if err := m.reload(); err != nil {
		return m, err
	}
	return m, nil
}

func (m *model) reload() error {
	h, err := listHosts(m.db)
	if err != nil {
		return err
	}
	m.allHosts = h
	m.applyFilter(false)
	return nil
}

func (m *model) applyFilter(resetCursor bool) {
	prevCursor := m.table.Cursor()
	q := strings.TrimSpace(m.search.Value())
	m.filteredIx = m.matchingIndices(q)
	rows := make([]table.Row, 0, len(m.filteredIx))
	for _, i := range m.filteredIx {
		h := m.allHosts[i]
		last := "-"
		if h.LastUsedAt.Valid {
			last = h.LastUsedAt.Time.Local().Format("2006-01-02 15:04")
		}
		rows = append(rows, table.Row{h.Host, h.Comment, last, fmt.Sprintf("%d", h.UseCount)})
	}
	m.table.SetRows(rows)
	if len(rows) == 0 {
		return
	}
	if resetCursor {
		m.table.SetCursor(0)
		return
	}
	if prevCursor < 0 {
		prevCursor = 0
	}
	if prevCursor >= len(rows) {
		prevCursor = len(rows) - 1
	}
	m.table.SetCursor(prevCursor)
}

func (m *model) matchingIndices(q string) []int {
	if q == "" {
		idx := make([]int, len(m.allHosts))
		for i := range m.allHosts {
			idx[i] = i
		}
		return idx
	}
	hay := make([]string, len(m.allHosts))
	for i, h := range m.allHosts {
		hay[i] = strings.ToLower(h.Host + " " + h.Comment)
	}
	res := fuzzy.Find(strings.ToLower(q), hay)
	sort.Slice(res, func(i, j int) bool {
		if res[i].Score == res[j].Score {
			li, lj := m.allHosts[res[i].Index], m.allHosts[res[j].Index]
			if li.LastUsedAt.Valid && lj.LastUsedAt.Valid {
				return li.LastUsedAt.Time.After(lj.LastUsedAt.Time)
			}
			if li.LastUsedAt.Valid != lj.LastUsedAt.Valid {
				return li.LastUsedAt.Valid
			}
			return li.Host < lj.Host
		}
		return res[i].Score > res[j].Score
	})
	idx := make([]int, 0, len(res))
	for _, r := range res {
		idx = append(idx, r.Index)
	}
	return idx
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		k := msg.String()
		switch m.mode {
		case modeList:
			switch k {
			case "ctrl+c", "q":
				return m, tea.Quit
			case "/":
				m.search.Focus()
				return m, nil
			case "esc":
				m.search.SetValue("")
				m.applyFilter(true)
				return m, nil
			case "up":
				if m.moveCursor(-1) {
					return m, nil
				}
			case "down":
				if m.moveCursor(1) {
					return m, nil
				}
			case "pgup":
				if m.movePage(-1) {
					return m, nil
				}
			case "pgdown", "pgdn":
				if m.movePage(1) {
					return m, nil
				}
			case "enter":
				if sel, ok := m.currentSelection(); ok {
					_ = markUsed(m.db, sel.ID)
					m.finalHost = sel.Host
					return m, tea.Quit // further action handled after Run()
				}
			case "ctrl+a", "alt+n":
				m.mode = modeAdd
				m.initInputs(Host{})
				return m, nil
			case "ctrl+e", "alt+e":
				if sel, ok := m.currentSelection(); ok {
					m.mode = modeEdit
					m.initInputs(sel)
				}
				return m, nil
			case "ctrl+d", "alt+d":
				if sel, ok := m.currentSelection(); ok {
					m.mode = modeConfirmDelete
					m.status = fmt.Sprintf("Delete %s? y/N", sel.Host)
					m.finalHost = ""
				}
				return m, nil
			case "ctrl+r", "alt+r":
				added, _ := importFromHistory(m.db)
				_ = setMeta(m.db, importDoneKey, "1")
				_ = m.reload()
				m.status = fmt.Sprintf("Imported from history: +%d", added)
				return m, nil
			}
		case modeAdd, modeEdit:
			switch k {
			case "esc":
				m.mode = modeList
				m.status = ""
				return m, nil
			case "enter":
				if m.inputs[0].Focused() {
					m.inputs[0].Blur()
					m.inputs[1].Focus()
					return m, nil
				}
				host := strings.TrimSpace(m.inputs[0].Value())
				comment := strings.TrimSpace(m.inputs[1].Value())
				if host == "" {
					m.status = "host cannot be empty"
					return m, nil
				}
				var err error
				if m.mode == modeAdd {
					err = addHost(m.db, host, comment)
				} else if sel, ok := m.currentSelection(); ok {
					err = updateHost(m.db, sel.ID, host, comment)
				}
				if err != nil {
					m.status = "error: " + err.Error()
					return m, nil
				}
				_ = m.reload()
				m.mode = modeList
				m.status = "saved"
				return m, nil
			}
			var cmds []tea.Cmd
			for i := range m.inputs {
				var c tea.Cmd
				m.inputs[i], c = m.inputs[i].Update(msg)
				cmds = append(cmds, c)
			}
			return m, tea.Batch(cmds...)
		case modeConfirmDelete:
			switch k {
			case "y", "Y":
				if sel, ok := m.currentSelection(); ok {
					_ = deleteHost(m.db, sel.ID)
					_ = m.reload()
					m.status = "deleted"
				}
				m.mode = modeList
				return m, nil
			case "n", "N", "esc", "enter":
				m.mode = modeList
				m.status = ""
				return m, nil
			}
		}
	}
	if m.mode == modeList {
		prevSearch := m.search.Value()
		var searchCmd tea.Cmd
		m.search, searchCmd = m.search.Update(msg)
		if m.search.Value() != prevSearch {
			m.applyFilter(true)
		}
		var tableCmd tea.Cmd
		m.table, tableCmd = m.table.Update(msg)
		return m, tea.Batch(searchCmd, tableCmd)
	}
	return m, nil
}

func (m model) View() string {
	switch m.mode {
	case modeAdd, modeEdit:
		t := "Add host"
		if m.mode == modeEdit {
			t = "Edit host"
		}
		return baseStyle.Render(headerStyle.Render(t) + "\n\n" + "Host:    " + m.inputs[0].View() + "\n" + "Comment: " + m.inputs[1].View() + "\n\n" + statusStyle.Render(m.status+"  (Enter: next/save, Esc: cancel)"))
	case modeConfirmDelete:
		return baseStyle.Render(headerStyle.Render("Confirm") + "\n\n" + statusStyle.Render(m.status))
	default:
		tableView := m.table.View()
		displayed := 0
		if tableView != "" {
			lines := strings.Split(tableView, "\n")
			if len(lines) > 1 {
				for _, line := range lines[1:] {
					if strings.TrimSpace(stripANSI(line)) != "" {
						displayed++
					}
				}
			}
		}
		infoLine := fmt.Sprintf("Total: %d  Matched: %d  Visible: %d", len(m.allHosts), len(m.filteredIx), displayed)
		return baseStyle.Render(
			headerStyle.Render("shh - SSH helper") + "\n" +
				m.search.View() + "\n\n" +
				tableView + "\n" +
				infoLine + "\n" +
				statusStyle.Render("Enter connect  / search  Ctrl+A/E/D add/edit/delete  Ctrl+R import  Ctrl+C or q quit") + "\n",
		)
	}
}

func (m *model) initInputs(h Host) {
	m.inputs = make([]textinput.Model, 2)
	for i := range m.inputs {
		m.inputs[i] = textinput.New()
	}
	m.inputs[0].Placeholder = "example.com"
	m.inputs[0].SetValue(h.Host)
	m.inputs[0].CharLimit = 256
	m.inputs[0].Focus()
	m.inputs[1].Placeholder = "description (optional)"
	m.inputs[1].SetValue(h.Comment)
	m.inputs[1].CharLimit = 512
}

func (m *model) currentSelection() (Host, bool) {
	if len(m.filteredIx) == 0 {
		return Host{}, false
	}
	row := m.table.Cursor()
	if row < 0 || row >= len(m.filteredIx) {
		return Host{}, false
	}
	return m.allHosts[m.filteredIx[row]], true
}

func (m *model) moveCursor(delta int) bool {
	if len(m.filteredIx) == 0 || delta == 0 {
		return len(m.filteredIx) > 0
	}
	if delta < 0 {
		m.table.MoveUp(-delta)
	} else {
		m.table.MoveDown(delta)
	}
	return true
}

func (m *model) movePage(delta int) bool {
	if len(m.filteredIx) == 0 || delta == 0 {
		return len(m.filteredIx) > 0
	}
	step := m.pageSize
	if step <= 0 {
		step = 1
	}
	if delta < 0 {
		m.table.MoveUp(-delta * step)
	} else {
		m.table.MoveDown(delta * step)
	}
	return true
}

func stripANSI(s string) string {
	return ansiRegexp.ReplaceAllString(s, "")
}

// ------------------------ shell exec ----------------------

var safeHost = regexp.MustCompile(`^(?:[A-Za-z0-9._-]+|\[[0-9A-Fa-f:]+\])$`)

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

func execInUserShellLogin(host string) {
	if !safeHost.MatchString(host) {
		log.Fatalf("invalid host: %q", host)
	}
	cleanupTerminal()
	if runtime.GOOS == "windows" {
		cmd := exec.Command("ssh", host)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				os.Exit(exitErr.ExitCode())
			}
			log.Fatalf("ssh command failed: %v", err)
		}
		return
	}
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash"
	}
	base := filepath.Base(shell)
	cmd := fmt.Sprintf("exec ssh %s", shellQuote(host))
	var argv []string
	switch base {
	case "bash", "zsh", "fish":
		argv = []string{shell, "-l", "-i", "-c", cmd}
	default:
		argv = []string{shell, "-i", "-c", cmd}
	}
	if err := syscall.Exec(shell, argv, os.Environ()); err != nil {
		log.Fatalf("failed to exec command: %v", err)
	}
}

func cleanupTerminal() {
	restoreConsoleState()
	if !isTerminal(os.Stdout) {
		return
	}
	// Reset basic attributes and ensure the cursor is visible.
	fmt.Fprint(os.Stdout, "\x1b[0m\x1b[?25h")
}

func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

// --------------------------- main ------------------------

func main() {
	captureConsoleState()
	defer cleanupTerminal()

	// run modes
	var showVersion bool
	flag.BoolVar(&showVersion, "version", false, "print version and exit")
	flag.BoolVar(&showVersion, "v", false, "print version and exit")
	printHost := flag.Bool("print", false, "print selected host and exit")
	printCmd := flag.Bool("cmd", false, "print command 'ssh <host>' and exit")
	flag.Parse()

	if showVersion {
		fmt.Println(version)
		return
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
		log.Fatalf("db error: %v", err)
	}
	defer db.Close()

	if v, ok, _ := getMeta(db, importDoneKey); !ok || v != "1" {
		if n, _ := importFromHistory(db); n > 0 {
			fmt.Fprintf(os.Stderr, "Imported from history: %d hosts\n", n)
		}
		_ = setMeta(db, importDoneKey, "1")
	}

	m, err := newModel(db, mode)
	if err != nil {
		log.Fatalf("init error: %v", err)
	}
	opts := []tea.ProgramOption{}
	if runtime.GOOS != "windows" {
		opts = append(opts, tea.WithAltScreen())
	}
	p := tea.NewProgram(m, opts...)
	res, err := p.Run()
	if err != nil {
		log.Fatalf("run error: %v", err)
	}
	mm := res.(model)
	if mm.finalHost == "" {
		return
	}

	switch mode {
	case runExecShell:
		execInUserShellLogin(mm.finalHost)
	case runPrintHost:
		fmt.Println(mm.finalHost)
	case runPrintCmd:
		fmt.Printf("ssh %s\n", mm.finalHost)
	}
}
