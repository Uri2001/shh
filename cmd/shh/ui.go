package main

import (
	"context"
	"fmt"
	"sort"
	"strings"

	table "github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sahilm/fuzzy"
)

type mode int

const (
	modeList mode = iota
	modeAdd
	modeEdit
	modeConfirmDelete
)

type runMode int

const (
	runExecShell runMode = iota
	runPrintHost
	runPrintCmd
)

type listView struct {
	table    table.Model
	search   textinput.Model
	pageSize int
}

type formView struct {
	inputs []textinput.Model
}

type confirmView struct {
	prompt string
}

type model struct {
	ctx        context.Context
	store      *Store
	mode       mode
	rmode      runMode
	list       listView
	form       formView
	confirm    confirmView
	status     string
	allHosts   []Host
	filteredIx []int
	finalHost  string
	width      int
	height     int
}

var (
	baseStyle   = lipgloss.NewStyle().Padding(0, 1).Foreground(lipgloss.Color("7"))
	headerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("10"))
	statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
)

func newListView() listView {
	search := textinput.New()
	search.Placeholder = "search (host/comment), / to focus, Esc to clear"
	search.Prompt = "/ "
	search.CharLimit = 256
	search.Focus()
	search.PromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
	search.TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))

	tbl := table.New(table.WithColumns(defaultColumns()), table.WithHeight(15))
	padding := lipgloss.NewStyle().Padding(0, 1)
	styles := table.DefaultStyles()
	styles.Header = padding.Copy().Bold(true).Foreground(lipgloss.Color("10"))
	styles.Cell = padding.Copy()
	styles.Selected = lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Bold(true)
	tbl.SetStyles(styles)

	l := listView{table: tbl, search: search}
	l.updatePageSize()
	return l
}

func (l *listView) updatePageSize() {
	l.pageSize = l.table.Height()
	if l.pageSize <= 0 {
		l.pageSize = 1
	}
}

func (l *listView) applyLayout(width, height int) {
	if width > 0 {
		l.search.Width = max(20, width-6)
		l.table.SetColumns(responsiveColumns(width))
	}
	if height > 0 {
		tableHeight := height - 7
		if tableHeight < 5 {
			tableHeight = 5
		}
		l.table.SetHeight(tableHeight)
	}
	l.updatePageSize()
}

func (l *listView) moveCursor(delta int) {
	if delta < 0 {
		l.table.MoveUp(-delta)
	} else if delta > 0 {
		l.table.MoveDown(delta)
	}
}

func (l *listView) movePage(delta int) {
	step := l.pageSize
	if step <= 0 {
		step = 1
	}
	if delta < 0 {
		l.table.MoveUp(-delta * step)
	} else if delta > 0 {
		l.table.MoveDown(delta * step)
	}
}

func newFormView() formView {
	inputs := make([]textinput.Model, 2)
	for i := range inputs {
		inputs[i] = textinput.New()
	}
	inputs[0].Placeholder = "example.com"
	inputs[0].CharLimit = 256
	inputs[0].Focus()
	inputs[1].Placeholder = "description (optional)"
	inputs[1].CharLimit = 512
	return formView{inputs: inputs}
}

func (f *formView) setHost(h Host) {
	if len(f.inputs) != 2 {
		return
	}
	f.inputs[0].SetValue(h.Host)
	f.inputs[0].Focus()
	f.inputs[1].Blur()
	f.inputs[1].SetValue(h.Comment)
}

func (f *formView) updateInputs(msg tea.Msg) []tea.Cmd {
	cmds := make([]tea.Cmd, 0, len(f.inputs))
	for i := range f.inputs {
		var cmd tea.Cmd
		f.inputs[i], cmd = f.inputs[i].Update(msg)
		cmds = append(cmds, cmd)
	}
	return cmds
}

func newModel(ctx context.Context, store *Store, r runMode) (model, error) {
	m := model{
		ctx:   ctx,
		store: store,
		mode:  modeList,
		rmode: r,
		list:  newListView(),
		form:  newFormView(),
	}
	if err := m.reload(); err != nil {
		return m, err
	}
	return m, nil
}

func (m *model) reload() error {
	hosts, err := m.store.ListHosts(m.ctx)
	if err != nil {
		return err
	}
	m.allHosts = hosts
	m.applyFilter(false)
	return nil
}

func (m *model) applyFilter(resetCursor bool) {
	prevCursor := m.list.table.Cursor()
	query := strings.TrimSpace(m.list.search.Value())
	m.filteredIx = m.matchingIndices(query)
	rows := make([]table.Row, 0, len(m.filteredIx))
	for _, idx := range m.filteredIx {
		h := m.allHosts[idx]
		last := "-"
		if h.LastUsedAt.Valid {
			last = h.LastUsedAt.Time.Local().Format("2006-01-02 15:04")
		}
		rows = append(rows, table.Row{h.Host, h.Comment, last, fmt.Sprintf("%d", h.UseCount)})
	}
	m.list.table.SetRows(rows)
	if len(rows) == 0 {
		return
	}
	if resetCursor {
		m.list.table.SetCursor(0)
		return
	}
	if prevCursor < 0 {
		prevCursor = 0
	}
	if prevCursor >= len(rows) {
		prevCursor = len(rows) - 1
	}
	m.list.table.SetCursor(prevCursor)
}

func (m *model) matchingIndices(query string) []int {
	if query == "" {
		idx := make([]int, len(m.allHosts))
		for i := range m.allHosts {
			idx[i] = i
		}
		return idx
	}
	haystack := make([]string, len(m.allHosts))
	for i, h := range m.allHosts {
		haystack[i] = strings.ToLower(h.Host + " " + h.Comment)
	}
	matches := fuzzy.Find(strings.ToLower(query), haystack)
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Score == matches[j].Score {
			hi, hj := m.allHosts[matches[i].Index], m.allHosts[matches[j].Index]
			if hi.LastUsedAt.Valid && hj.LastUsedAt.Valid {
				return hi.LastUsedAt.Time.After(hj.LastUsedAt.Time)
			}
			if hi.LastUsedAt.Valid != hj.LastUsedAt.Valid {
				return hi.LastUsedAt.Valid
			}
			return hi.Host < hj.Host
		}
		return matches[i].Score > matches[j].Score
	})
	idx := make([]int, 0, len(matches))
	for _, match := range matches {
		idx = append(idx, match.Index)
	}
	return idx
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.list.applyLayout(msg.Width, msg.Height)
		return m, nil
	case tea.KeyMsg:
		switch m.mode {
		case modeList:
			var cmd tea.Cmd
			var handled bool
			m, cmd, handled = m.handleListKey(msg)
			if handled {
				return m, cmd
			}
		case modeAdd, modeEdit:
			return m.handleFormKey(msg)
		case modeConfirmDelete:
			return m.handleConfirmKey(msg)
		}
	}

	switch m.mode {
	case modeList:
		prev := m.list.search.Value()
		var searchCmd tea.Cmd
		m.list.search, searchCmd = m.list.search.Update(msg)
		if m.list.search.Value() != prev {
			m.applyFilter(true)
		}
		var tableCmd tea.Cmd
		m.list.table, tableCmd = m.list.table.Update(msg)
		return m, tea.Batch(searchCmd, tableCmd)
	case modeAdd, modeEdit:
		cmds := m.form.updateInputs(msg)
		return m, tea.Batch(cmds...)
	default:
		return m, nil
	}
}

func (m model) handleListKey(msg tea.KeyMsg) (model, tea.Cmd, bool) {
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit, true
	case "/":
		m.list.search.Focus()
		return m, nil, true
	case "esc":
		m.list.search.SetValue("")
		m.applyFilter(true)
		return m, nil, true
	case "up":
		if len(m.filteredIx) == 0 {
			return m, nil, true
		}
		m.list.moveCursor(-1)
		return m, nil, true
	case "down":
		if len(m.filteredIx) == 0 {
			return m, nil, true
		}
		m.list.moveCursor(1)
		return m, nil, true
	case "pgup":
		if len(m.filteredIx) == 0 {
			return m, nil, true
		}
		m.list.movePage(-1)
		return m, nil, true
	case "pgdown", "pgdn":
		if len(m.filteredIx) == 0 {
			return m, nil, true
		}
		m.list.movePage(1)
		return m, nil, true
	case "enter":
		if sel, ok := m.currentSelection(); ok {
			if err := m.store.MarkUsed(m.ctx, sel.ID); err != nil {
				m.status = "mark used: " + err.Error()
			} else {
				m.status = ""
			}
			m.finalHost = sel.Host
			return m, tea.Quit, true
		}
	case "ctrl+a", "alt+n":
		m.mode = modeAdd
		m.form.setHost(Host{})
		m.status = ""
		return m, nil, true
	case "ctrl+e", "alt+e":
		if sel, ok := m.currentSelection(); ok {
			m.mode = modeEdit
			m.form.setHost(sel)
			m.status = ""
		}
		return m, nil, true
	case "ctrl+d", "alt+d":
		if sel, ok := m.currentSelection(); ok {
			m.mode = modeConfirmDelete
			m.confirm.prompt = fmt.Sprintf("Delete %s? y/N", sel.Host)
			m.status = ""
			m.finalHost = ""
		}
		return m, nil, true
	case "ctrl+r", "alt+r":
		if added, err := m.store.ImportFromHistory(m.ctx); err != nil {
			m.status = "import error: " + err.Error()
		} else {
			if err := m.store.SetMeta(m.ctx, importDoneKey, "1"); err != nil {
				m.status = "meta error: " + err.Error()
			} else if err := m.reload(); err != nil {
				m.status = "reload error: " + err.Error()
			} else {
				m.status = fmt.Sprintf("Imported from history: +%d", added)
			}
		}
		return m, nil, true
	}
	return m, nil, false
}

func (m model) handleFormKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeList
		m.status = ""
		return m, nil
	case "enter":
		if m.form.inputs[0].Focused() {
			m.form.inputs[0].Blur()
			m.form.inputs[1].Focus()
			return m, nil
		}
		host := m.form.inputs[0].Value()
		comment := m.form.inputs[1].Value()
		var err error
		if m.mode == modeAdd {
			err = m.store.AddHost(m.ctx, host, comment)
		} else if sel, ok := m.currentSelection(); ok {
			err = m.store.UpdateHost(m.ctx, sel.ID, host, comment)
		}
		if err != nil {
			m.status = "error: " + err.Error()
			return m, nil
		}
		if err := m.reload(); err != nil {
			m.status = "reload error: " + err.Error()
			return m, nil
		}
		m.mode = modeList
		m.status = "saved"
		return m, nil
	}
	cmds := m.form.updateInputs(msg)
	return m, tea.Batch(cmds...)
}

func (m model) handleConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		if sel, ok := m.currentSelection(); ok {
			if err := m.store.DeleteHost(m.ctx, sel.ID); err != nil {
				m.status = "delete error: " + err.Error()
			} else if err := m.reload(); err != nil {
				m.status = "reload error: " + err.Error()
			} else {
				m.status = "deleted"
			}
		}
		m.mode = modeList
		return m, nil
	case "n", "N", "esc", "enter":
		m.mode = modeList
		m.status = ""
		return m, nil
	}
	return m, nil
}

func (m model) View() string {
	switch m.mode {
	case modeAdd, modeEdit:
		title := "Add host"
		if m.mode == modeEdit {
			title = "Edit host"
		}
		return baseStyle.Render(
			headerStyle.Render(title) + "\n\n" +
				"Host:    " + m.form.inputs[0].View() + "\n" +
				"Comment: " + m.form.inputs[1].View() + "\n\n" +
				statusStyle.Render(m.status+"  (Enter: next/save, Esc: cancel)"),
		)
	case modeConfirmDelete:
		return baseStyle.Render(
			headerStyle.Render("Confirm") + "\n\n" +
				statusStyle.Render(m.confirm.prompt),
		)
	default:
		tableView := m.list.table.View()
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
		footer := statusStyle.Render("Enter connect  / search  Ctrl+A/E/D add/edit/delete  Ctrl+R import  Ctrl+C or q quit")
		if m.status != "" {
			footer += "\n" + statusStyle.Render(m.status)
		}
		return baseStyle.Render(
			headerStyle.Render("shh - SSH helper") + "\n" +
				m.list.search.View() + "\n\n" +
				tableView + "\n" +
				infoLine + "\n" +
				footer,
		)
	}
}

func (m *model) currentSelection() (Host, bool) {
	if len(m.filteredIx) == 0 {
		return Host{}, false
	}
	row := m.list.table.Cursor()
	if row < 0 || row >= len(m.filteredIx) {
		return Host{}, false
	}
	return m.allHosts[m.filteredIx[row]], true
}

func stripANSI(s string) string {
	return ansiRegexp.ReplaceAllString(s, "")
}

func defaultColumns() []table.Column {
	return []table.Column{
		{Title: "Host", Width: 36},
		{Title: "Comment", Width: 60},
		{Title: "Last Used", Width: 19},
		{Title: "#", Width: 4},
	}
}

func responsiveColumns(width int) []table.Column {
	const (
		lastUsedWidth = 19
		countWidth    = 4
		minHostWidth  = 16
		minComment    = 16
		padding       = 8
	)
	available := max(minHostWidth+minComment, width-padding-lastUsedWidth-countWidth)
	hostWidth := available / 2
	commentWidth := available - hostWidth

	if hostWidth < minHostWidth {
		hostWidth = minHostWidth
		commentWidth = available - hostWidth
	}
	if commentWidth < minComment {
		commentWidth = minComment
		hostWidth = available - commentWidth
		if hostWidth < minHostWidth {
			hostWidth = minHostWidth
		}
	}

	return []table.Column{
		{Title: "Host", Width: hostWidth},
		{Title: "Comment", Width: commentWidth},
		{Title: "Last Used", Width: lastUsedWidth},
		{Title: "#", Width: countWidth},
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
