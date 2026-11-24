package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const startDir = "/Users/edwinr/Documents/Projects/Prgraming_01/01_testrepos"

// ---------- messages ----------
type scanFinishedMsg struct {
	files map[string]string
	err   error
}
type blinkTickMsg struct{}
type delayDoneMsg struct{}

// ---------- model ----------
type focus int

const (
	focusLeft focus = iota
	focusRight
)

type model struct {
	cwd       string
	entries   []os.DirEntry
	cursor    int
	selected  map[string]bool // extension -> checked
	focus     focus
	table     table.Model
	spinner   spinner.Model
	status    string
	help      string
	scanning  bool
	done      bool
	err       error
	outDir    string
	showAnim  bool
	blinkAlt  bool
	blinkDone bool
}

func newModel() model {
	wd, _ := os.Getwd()
	outDir := filepath.Join(wd, "result")
	_ = os.MkdirAll(outDir, 0755)

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	m := model{
		cwd:      startDir,
		selected: map[string]bool{},
		outDir:   outDir,
		status:   "",
		help:     "Enter=folder  Tab=switch pane  Space=toggle ext  s=scan  q=quit",
		spinner:  s,
	}
	m.refresh()
	return m
}

// refresh reloads entries and rebuilds the extension table
func (m *model) refresh() {
	entries, err := os.ReadDir(m.cwd)
	if err != nil {
		m.status = "read error: " + err.Error()
		return
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir() != entries[j].IsDir() {
			return entries[i].IsDir()
		}
		return entries[i].Name() < entries[j].Name()
	})
	m.entries = append([]os.DirEntry{upEntry{}}, entries...)
	if m.cursor >= len(m.entries) {
		m.cursor = 0
	}

	extSet := map[string]bool{}
	_ = filepath.WalkDir(m.cwd, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && (d.Name() == ".git" || d.Name() == "node_modules") {
			return filepath.SkipDir
		}
		if !d.IsDir() {
			e := strings.ToLower(filepath.Ext(p))
			if e != "" && e != ".ds_store" {
				extSet[e] = true
			}
		}
		return nil
	})
	var exts []string
	for e := range extSet {
		exts = append(exts, e)
	}
	sort.Strings(exts)

	rows := make([]table.Row, 0, len(exts))
	for _, e := range exts {
		mark := "[ ]"
		if m.selected[e] {
			mark = "[x]"
		}
		rows = append(rows, table.Row{mark, e})
	}
	cols := []table.Column{
		{Title: "Inc", Width: 4},
		{Title: "Ext", Width: 12},
	}
	t := table.New(
		table.WithColumns(cols),
		table.WithRows(rows),
		table.WithFocused(false),
		table.WithHeight(12),
	)
	m.table = t
}

type upEntry struct{}

func (upEntry) Name() string               { return ".." }
func (upEntry) IsDir() bool                { return true }
func (upEntry) Type() os.FileMode          { return os.ModeDir }
func (upEntry) Info() (os.FileInfo, error) { return nil, nil }

func (m model) Init() tea.Cmd { return m.spinner.Tick }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case tea.KeyMsg:
		k := msg.String()
		if k == "q" || k == "ctrl+c" {
			return m, tea.Quit
		}
		if k == "tab" {
			if m.focus == focusLeft {
				m.focus = focusRight
				m.table.Focus()
			} else {
				m.focus = focusLeft
				m.table.Blur()
			}
			return m, nil
		}
		if m.scanning {
			return m, nil
		}
		if m.focus == focusLeft {
			switch k {
			case "up", "k":
				if m.cursor > 0 {
					m.cursor--
				}
			case "down", "j":
				if m.cursor < len(m.entries)-1 {
					m.cursor++
				}
			case "enter":
				e := m.entries[m.cursor]
				if e.IsDir() {
					if e.Name() == ".." {
						m.cwd = filepath.Dir(m.cwd)
					} else {
						m.cwd = filepath.Join(m.cwd, e.Name())
					}
					m.cursor = 0
					m.refresh()
				}
			case "s":
				if len(m.selected) == 0 {
					m.status = "No extensions selected"
					return m, nil
				}
				m.scanning = true
				m.done = false
				m.showAnim = false
				m.status = "Scanning..."
				return m, scanCmd(m.cwd, m.selected, m.outDir)
			}
		} else {
			switch k {
			case "up", "k":
				m.table.MoveUp(1)
			case "down", "j":
				m.table.MoveDown(1)
			case " ":
				row := m.table.SelectedRow()
				if len(row) == 2 {
					ext := row[1]
					m.selected[ext] = !m.selected[ext]
					saved := m.table.Cursor()
					m.refresh()
					m.table.SetCursor(saved)
				}
			case "s":
				if len(m.selected) == 0 {
					m.status = "No extensions selected"
					return m, nil
				}
				m.scanning = true
				m.done = false
				m.showAnim = false
				m.status = "Scanning..."
				return m, scanCmd(m.cwd, m.selected, m.outDir)
			}
		}

	case scanFinishedMsg:
		// Keep m.scanning = true so the spinner keeps rendering.
		m.done = true
		m.err = msg.err
		m.showAnim = false // success/error icon not yet
		if msg.err != nil {
			m.status = "Scan failed"
		} else {
			m.status = fmt.Sprintf("Wrote %d files to result", len(msg.files))
		}
		return m, tea.Tick(3*time.Second, func(time.Time) tea.Msg {
			return delayDoneMsg{}
		})

	case delayDoneMsg:
		m.scanning = false
		m.showAnim = true
		m.blinkAlt = false
		m.blinkDone = false
		return m, tea.Tick(500*time.Millisecond, func(time.Time) tea.Msg {
			return blinkTickMsg{}
		})

	case blinkTickMsg:
		if !m.showAnim {
			return m, nil
		}
		m.blinkAlt = !m.blinkAlt
		if m.blinkDone {
			return m, nil
		}
		return m, tea.Tick(500*time.Millisecond, func(time.Time) tea.Msg {
			return blinkTickMsg{}
		})
	}
	return m, nil
}

func (m model) View() string {
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205")).Render("Project Scanner")
	leftStyle := lipgloss.NewStyle().Border(lipgloss.NormalBorder()).Width(50).Height(16)
	rightStyle := lipgloss.NewStyle().Border(lipgloss.NormalBorder()).Width(30).Height(16)

	var left strings.Builder
	left.WriteString("CWD: " + m.cwd + "\n\n")
	for i, e := range m.entries {
		marker := "  "
		if i == m.cursor {
			marker = "> "
		}
		name := e.Name()
		if e.IsDir() {
			name += "/"
		}
		left.WriteString(marker + name + "\n")
		if i > 30 {
			left.WriteString("...\n")
			break
		}
	}
	right := "Extensions\n\n" + m.table.View()

	l := leftStyle.Render(left.String())
	r := rightStyle.Render(right)
	if m.focus == focusLeft {
		l = lipgloss.NewStyle().Border(lipgloss.ThickBorder()).Width(50).Height(16).Render(left.String())
	} else {
		r = lipgloss.NewStyle().Border(lipgloss.ThickBorder()).Width(30).Height(16).Render(right)
	}

	var footer string
	switch {
	case m.scanning:
		footer = m.spinner.View() + " Scanning…"
	case m.showAnim && m.err != nil:
		icon := ":("
		if m.blinkAlt {
			icon = ":X"
		}
		footer = "Scan failed " + icon + " — " + m.err.Error()
	case m.showAnim:
		icon := ":d"
		if m.blinkAlt {
			icon = ":D"
		}
		footer = "Scanning complete " + icon + " — " + m.status
	default:
		footer = m.status
	}

	return title + "\n\n" +
		lipgloss.JoinHorizontal(lipgloss.Top, l, r) + "\n\n" +
		m.help + "\n" +
		footer + "\n"
}

// ---------- scan command ----------
func scanCmd(root string, selected map[string]bool, outDir string) tea.Cmd {
	return func() tea.Msg {
		files := map[string]string{}
		err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() && (d.Name() == ".git" || d.Name() == "node_modules") {
				return filepath.SkipDir
			}
			if d.IsDir() {
				return nil
			}
			ext := strings.ToLower(filepath.Ext(p))
			if selected[ext] {
				if isLikelyBinary(p) {
					return nil
				}
				data, readErr := os.ReadFile(p)
				if readErr == nil {
					files[p] = string(data)
				}
			}
			return nil
		})
		if err != nil {
			return scanFinishedMsg{err: err}
		}
		if err := writeOutputs(root, files, outDir); err != nil {
			return scanFinishedMsg{err: err}
		}
		return scanFinishedMsg{files: files}
	}
}

func writeOutputs(root string, files map[string]string, outDir string) error {
	var buf strings.Builder
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		rel, _ := filepath.Rel(root, p)
		fmt.Fprintf(&buf, "=== %s ===\n%s\n\n", rel, files[p])
	}
	if err := os.WriteFile(filepath.Join(outDir, "consolidated.txt"), []byte(buf.String()), 0644); err != nil {
		return err
	}

	var tree strings.Builder
	tree.WriteString(filepath.Base(root) + "\n")
	seen := map[string]bool{}
	for _, p := range paths {
		rel, _ := filepath.Rel(root, p)
		parts := strings.Split(rel, string(filepath.Separator))
		acc := ""
		for i, part := range parts {
			if i == len(parts)-1 {
				break
			}
			acc = filepath.Join(acc, part)
			if !seen[acc] {
				seen[acc] = true
				tree.WriteString(strings.Repeat("│   ", i) + "├── " + part + "/\n")
			}
		}
		tree.WriteString(strings.Repeat("│   ", len(parts)-1) + "└── " + parts[len(parts)-1] + "\n")
	}
	return os.WriteFile(filepath.Join(outDir, "tree.txt"), []byte(tree.String()), 0644)
}

// isLikelyBinary returns true if the file's first 512 bytes contain a NUL byte,
// the same heuristic git uses to detect binary files.
func isLikelyBinary(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return true
	}
	defer f.Close()
	buf := make([]byte, 512)
	n, _ := f.Read(buf)
	for i := 0; i < n; i++ {
		if buf[i] == 0 {
			return true
		}
	}
	return false
}

func main() {
	m := newModel()
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "TUI error:", err)
		os.Exit(1)
	}
}
