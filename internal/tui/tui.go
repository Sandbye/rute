// Package tui implements the interactive Bubble Tea TUI browser.
package tui

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sandbye/rute/internal/parser"
	"github.com/sandbye/rute/internal/renderer"
	ruteYaml "github.com/sandbye/rute/internal/yaml"
)

// Run launches the interactive TUI browser for the given config.
func Run(cfg *ruteYaml.Config, baseDir string) error {
	m := newModel(cfg, baseDir)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

// ---------------------------------------------------------------------------
// Sidebar items — mix of group labels and endpoint entries
// ---------------------------------------------------------------------------

type sidebarItem struct {
	isGroup bool
	label   string // group name (if isGroup)
	epIndex int    // index into cfg.Endpoints (if !isGroup)
}

func buildSidebarItems(cfg *ruteYaml.Config) []sidebarItem {
	type group struct {
		name    string
		indices []int
	}

	grouped := make(map[string]*group)
	order := []string{}

	for i, ep := range cfg.Endpoints {
		var gname string
		if len(ep.Tags) > 0 {
			gname = ep.Tags[0]
		} else {
			parts := strings.Split(strings.TrimPrefix(ep.Path, "/"), "/")
			if len(parts) > 0 {
				gname = parts[0]
			} else {
				gname = "other"
			}
		}

		if _, ok := grouped[gname]; !ok {
			order = append(order, gname)
			grouped[gname] = &group{name: gname}
		}
		grouped[gname].indices = append(grouped[gname].indices, i)
	}

	items := []sidebarItem{}
	for _, name := range order {
		g := grouped[name]
		label := strings.ToUpper(name[:1]) + name[1:]
		items = append(items, sidebarItem{isGroup: true, label: label})
		for _, idx := range g.indices {
			items = append(items, sidebarItem{epIndex: idx})
		}
	}
	return items
}

// ---------------------------------------------------------------------------
// Model
// ---------------------------------------------------------------------------

type model struct {
	cfg     *ruteYaml.Config
	baseDir string

	sidebar []sidebarItem
	cursor  int // cursor indexes into sidebar (skips groups)
	scroll  int // scroll offset for the detail pane
	width   int
	height  int
	details map[int]string // cached rendered detail per endpoint index

	// Search state
	searching    bool
	searchQuery  string
	searchResult []int // indices into cfg.Endpoints that match
	searchCursor int   // cursor within searchResult
}

func newModel(cfg *ruteYaml.Config, baseDir string) model {
	items := buildSidebarItems(cfg)
	cursor := 0
	for i, item := range items {
		if !item.isGroup {
			cursor = i
			break
		}
	}
	return model{
		cfg:     cfg,
		baseDir: baseDir,
		sidebar: items,
		cursor:  cursor,
		details: make(map[int]string),
	}
}

func (m model) Init() tea.Cmd {
	return nil
}

// selectedEndpointIndex returns the cfg.Endpoints index for the current cursor.
func (m model) selectedEndpointIndex() int {
	if m.searching && len(m.searchResult) > 0 {
		return m.searchResult[m.searchCursor]
	}
	if m.cursor >= 0 && m.cursor < len(m.sidebar) {
		return m.sidebar[m.cursor].epIndex
	}
	return 0
}

// ---------------------------------------------------------------------------
// Search
// ---------------------------------------------------------------------------

func (m *model) updateSearch() {
	q := strings.ToLower(m.searchQuery)
	m.searchResult = nil
	if q == "" {
		return
	}
	for i, ep := range m.cfg.Endpoints {
		haystack := strings.ToLower(ep.Method + " " + ep.Path + " " + ep.Description)
		if strings.Contains(haystack, q) {
			m.searchResult = append(m.searchResult, i)
		}
	}
	m.searchCursor = 0
}

func (m *model) selectSearchResult() {
	if len(m.searchResult) == 0 {
		return
	}
	epIdx := m.searchResult[m.searchCursor]
	// Find the sidebar item for this endpoint index and move cursor there.
	for i, item := range m.sidebar {
		if !item.isGroup && item.epIndex == epIdx {
			m.cursor = i
			break
		}
	}
	m.scroll = 0
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.searching {
			return m.updateSearch_mode(msg)
		}
		return m.updateNormal(msg)
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}
	return m, nil
}

func (m model) updateNormal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "/":
		m.searching = true
		m.searchQuery = ""
		m.searchResult = nil
		m.searchCursor = 0
	case "up", "k":
		m.moveCursor(-1)
		m.scroll = 0
	case "down", "j":
		m.moveCursor(1)
		m.scroll = 0
	case "pgup", "ctrl+u":
		m.scroll -= m.height / 2
		if m.scroll < 0 {
			m.scroll = 0
		}
	case "pgdown", "ctrl+d":
		m.scroll += m.height / 2
	}
	return m, nil
}

func (m model) updateSearch_mode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.searching = false
		m.searchQuery = ""
		m.searchResult = nil
	case "enter":
		m.selectSearchResult()
		m.searching = false
		m.searchQuery = ""
		m.searchResult = nil
	case "up", "ctrl+p":
		if m.searchCursor > 0 {
			m.searchCursor--
		}
	case "down", "ctrl+n":
		if m.searchCursor < len(m.searchResult)-1 {
			m.searchCursor++
		}
	case "backspace":
		if len(m.searchQuery) > 0 {
			m.searchQuery = m.searchQuery[:len(m.searchQuery)-1]
			m.updateSearch()
		}
	default:
		// Only accept printable characters
		if len(msg.String()) == 1 && msg.String()[0] >= 32 {
			m.searchQuery += msg.String()
			m.updateSearch()
		}
	}
	return m, nil
}

// moveCursor moves to the next/prev non-group sidebar item.
func (m *model) moveCursor(dir int) {
	next := m.cursor + dir
	for next >= 0 && next < len(m.sidebar) {
		if !m.sidebar[next].isGroup {
			m.cursor = next
			return
		}
		next += dir
	}
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

var (
	sidebarPad = lipgloss.NewStyle().
			Padding(1, 1)

	detailPad = lipgloss.NewStyle().
			Padding(1, 2)

	selectedBg = lipgloss.NewStyle().
			Bold(true).
			Background(lipgloss.Color("8")).
			Foreground(lipgloss.Color("15"))

	titleLook = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("12")).
			Padding(0, 0, 1, 0)

	groupLook = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8")).
			Bold(true).
			Padding(1, 0, 0, 0)

	sectionLook = lipgloss.NewStyle().
			Foreground(lipgloss.Color("12")).
			Bold(true)

	helpLook = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8"))

	borderLook = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), false, true, false, false).
			BorderForeground(lipgloss.Color("8"))

	searchInputStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("15")).
				Bold(true)

	searchPromptStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("12")).
				Bold(true)

	searchMatchStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("15"))

	searchSelectedStyle = lipgloss.NewStyle().
				Bold(true).
				Background(lipgloss.Color("12")).
				Foreground(lipgloss.Color("15"))

	searchDimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8"))
)

func (m model) View() string {
	if m.width == 0 || m.height == 0 {
		return "Loading..."
	}

	sideWidth := m.width * 30 / 100
	if sideWidth < 20 {
		sideWidth = 20
	}
	if sideWidth > 50 {
		sideWidth = 50
	}
	detailWidth := m.width - sideWidth - 3

	// Detail pane shows the selected endpoint (even during search, live preview)
	detail := m.renderDetail(detailWidth, m.height-2)

	var sidebar string
	if m.searching {
		sidebar = m.renderSearchOverlay(sideWidth, m.height-2)
	} else {
		sidebar = m.renderSidebar(sideWidth, m.height-2)
	}

	sidebar = borderLook.Width(sideWidth).Height(m.height - 2).Render(sidebar)

	main := lipgloss.JoinHorizontal(lipgloss.Top, sidebar, detail)

	var help string
	if m.searching {
		help = helpLook.Render("  ↑/↓ select  •  enter jump  •  esc close")
	} else {
		help = helpLook.Render("  ↑/↓ navigate  •  / search  •  pgup/pgdn scroll  •  q quit")
	}

	return main + "\n" + help
}

func (m model) renderSidebar(width, height int) string {
	var sb strings.Builder

	sb.WriteString(titleLook.Render(m.cfg.Title))
	sb.WriteString("\n")

	listHeight := height - 3
	lineCount := 0

	for i, item := range m.sidebar {
		if lineCount >= listHeight {
			break
		}

		if item.isGroup {
			sb.WriteString(groupLook.Render(strings.ToUpper(item.label)))
			sb.WriteString("\n")
			lineCount++
			continue
		}

		ep := m.cfg.Endpoints[item.epIndex]
		method := renderer.MethodStyle(fmt.Sprintf("%-6s", ep.Method))
		path := ep.Path

		if i == m.cursor {
			path = selectedBg.Render(path)
		}

		sb.WriteString(fmt.Sprintf("%s %s\n", method, path))
		lineCount++
	}

	return sidebarPad.Width(width - 2).Render(sb.String())
}

func (m model) renderSearchOverlay(width, height int) string {
	var sb strings.Builder

	// Search input
	prompt := searchPromptStyle.Render("/ ")
	query := searchInputStyle.Render(m.searchQuery)
	cursor := searchPromptStyle.Render("▋")
	sb.WriteString(prompt + query + cursor + "\n\n")

	if len(m.searchResult) == 0 && m.searchQuery != "" {
		sb.WriteString(searchDimStyle.Render("  No matches"))
		sb.WriteString("\n")
	}

	maxResults := height - 4
	for i, epIdx := range m.searchResult {
		if i >= maxResults {
			remaining := len(m.searchResult) - maxResults
			sb.WriteString(searchDimStyle.Render(fmt.Sprintf("  … %d more", remaining)))
			sb.WriteString("\n")
			break
		}

		ep := m.cfg.Endpoints[epIdx]
		method := renderer.MethodStyle(fmt.Sprintf("%-6s", ep.Method))
		path := ep.Path

		if i == m.searchCursor {
			path = searchSelectedStyle.Render(path)
		} else {
			path = searchMatchStyle.Render(path)
		}

		sb.WriteString(fmt.Sprintf("  %s %s\n", method, path))
	}

	return sidebarPad.Width(width - 2).Render(sb.String())
}

func (m model) renderDetail(width, height int) string {
	if len(m.cfg.Endpoints) == 0 {
		return detailPad.Render("No endpoints.")
	}

	epIdx := m.selectedEndpointIndex()
	detail := m.getDetail(epIdx)

	lines := strings.Split(detail, "\n")
	if m.scroll > len(lines)-1 {
		m.scroll = len(lines) - 1
	}
	if m.scroll < 0 {
		m.scroll = 0
	}
	if m.scroll < len(lines) {
		lines = lines[m.scroll:]
	}

	visibleHeight := height - 2
	if len(lines) > visibleHeight {
		lines = lines[:visibleHeight]
	}

	content := strings.Join(lines, "\n")
	return detailPad.Width(width).Render(content)
}

func (m model) getDetail(idx int) string {
	if cached, ok := m.details[idx]; ok {
		return cached
	}

	ep := m.cfg.Endpoints[idx]
	var sb strings.Builder

	heading := lipgloss.NewStyle().Bold(true)
	sb.WriteString(fmt.Sprintf("%s %s\n", renderer.MethodStyle(ep.Method), heading.Render(ep.Path)))
	if ep.Description != "" {
		sb.WriteString(ep.Description)
		sb.WriteString("\n")
	}
	sb.WriteString("\n")

	if ep.Params != nil && ep.Params.Schema != "" {
		sb.WriteString(sectionLook.Render("Params"))
		sb.WriteString("\n")
		m.writeSchema(&sb, ep.Params.Schema)
	}

	if ep.Query != nil && ep.Query.Schema != "" {
		sb.WriteString(sectionLook.Render("Query"))
		sb.WriteString("\n")
		m.writeSchema(&sb, ep.Query.Schema)
	}

	if ep.Body != nil && ep.Body.Schema != "" {
		sb.WriteString(sectionLook.Render("Body"))
		sb.WriteString("\n")
		m.writeSchema(&sb, ep.Body.Schema)
	}

	if len(ep.Response) > 0 {
		codes := make([]string, 0, len(ep.Response))
		for code := range ep.Response {
			codes = append(codes, code)
		}
		sort.Strings(codes)

		for _, code := range codes {
			holder := ep.Response[code]
			sb.WriteString(sectionLook.Render(fmt.Sprintf("Response %s", code)))
			sb.WriteString("\n")
			if holder.Schema != "" {
				m.writeSchema(&sb, holder.Schema)
			}
		}
	}

	result := sb.String()
	m.details[idx] = result
	return result
}

func (m model) writeSchema(sb *strings.Builder, ref string) {
	sr, err := ruteYaml.ParseSchemaRef(ref)
	if err != nil {
		sb.WriteString(fmt.Sprintf("  error: %v\n\n", err))
		return
	}
	absBase, _ := filepath.Abs(m.baseDir)
	schema, err := parser.Parse(absBase, sr.File, sr.Export)
	if err != nil {
		sb.WriteString(fmt.Sprintf("  error: %v\n\n", err))
		return
	}
	sb.WriteString(renderer.RenderSchema(schema, "", true))
	sb.WriteString("\n")
}
