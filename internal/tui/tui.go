// Package tui implements the interactive Bubble Tea TUI browser.
package tui

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sandbye/rute/internal/parser"
	"github.com/sandbye/rute/internal/renderer"
	ruteYaml "github.com/sandbye/rute/internal/yaml"
)

// Run launches the interactive TUI browser for the given config.
func Run(cfg *ruteYaml.Config, baseDir, binaryVersion string) error {
	m := newModel(cfg, baseDir, binaryVersion)
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
// Use mode field — represents a single editable input in use mode
// ---------------------------------------------------------------------------

type useModeField struct {
	section    string // "params", "query", "body", "headers"
	key        string // field name
	value      string // current value (kept in sync with input.Value())
	editing    bool   // editing the value
	editingKey bool   // editing the key (headers only)
	input      textinput.Model
	keyInput   textinput.Model // for header key editing

	// suggestion state (populated while editing)
	suggestions    []string // filtered suggestion list
	suggestionIdx  int      // -1 = none selected
	showSuggestion bool     // whether the dropdown is visible
}

// ---------------------------------------------------------------------------
// Async HTTP response message
// ---------------------------------------------------------------------------

type httpResponseMsg struct {
	epIndex    int
	method     string
	url        string
	statusCode int
	body       string
	err        error
	elapsed    time.Duration
}

type clipboardMsg struct {
	err error
}

// ---------------------------------------------------------------------------
// History
// ---------------------------------------------------------------------------

type historyEntry struct {
	epIndex  int
	fields   []useModeField
	response httpResponseMsg
	firedAt  time.Time
}

// persistedField is the JSON-serialisable form of useModeField.
type persistedField struct {
	Section string `json:"section"`
	Key     string `json:"key"`
	Value   string `json:"value"`
}

// persistedEntry is the JSON-serialisable form of historyEntry.
type persistedEntry struct {
	EpIndex    int              `json:"ep_index"`
	Fields     []persistedField `json:"fields"`
	Method     string           `json:"method,omitempty"`
	URL        string           `json:"url,omitempty"`
	StatusCode int              `json:"status_code"`
	Body       string           `json:"body"`
	ErrMsg     string           `json:"err_msg,omitempty"`
	ElapsedMs  int64            `json:"elapsed_ms"`
	FiredAt    time.Time        `json:"fired_at"`
}

// ---------------------------------------------------------------------------
// Project data directory — .rute/ next to rute.yaml
// ---------------------------------------------------------------------------

func ruteDataDir(baseDir string) (string, error) {
	dir := filepath.Join(baseDir, ".rute")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return dir, nil
}

func historyFilePath(baseDir string) string {
	dir, err := ruteDataDir(baseDir)
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "history.json")
}

func varsFilePath(baseDir string) string {
	dir, err := ruteDataDir(baseDir)
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "vars.json")
}

// ---------------------------------------------------------------------------
// Variables — {{NAME}} substitution, stored in .rute/vars.json
// ---------------------------------------------------------------------------

// ruteVar is a single named variable.
type ruteVar struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

func loadVars(baseDir string) []ruteVar {
	path := varsFilePath(baseDir)
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var vars []ruteVar
	if err := json.Unmarshal(data, &vars); err != nil {
		return nil
	}
	return vars
}

func saveVars(baseDir string, vars []ruteVar) {
	path := varsFilePath(baseDir)
	if path == "" {
		return
	}
	data, _ := json.MarshalIndent(vars, "", "  ")
	_ = os.WriteFile(path, data, 0644)
}

// resolveVars replaces all {{NAME}} occurrences in s using the provided vars.
func resolveVars(s string, vars []ruteVar) string {
	for _, v := range vars {
		s = strings.ReplaceAll(s, "{{"+v.Name+"}}", v.Value)
	}
	return s
}

// resolveFields returns a copy of fields with all values resolved.
func resolveFields(fields []useModeField, vars []ruteVar) []useModeField {
	out := make([]useModeField, len(fields))
	for i, f := range fields {
		f.value = resolveVars(f.value, vars)
		out[i] = f
	}
	return out
}

func loadHistory(baseDir string) []historyEntry {
	path := historyFilePath(baseDir)
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var persisted []persistedEntry
	if err := json.Unmarshal(data, &persisted); err != nil {
		return nil
	}
	entries := make([]historyEntry, 0, len(persisted))
	for _, p := range persisted {
		fields := make([]useModeField, len(p.Fields))
		for i, f := range p.Fields {
			fields[i] = useModeField{section: f.Section, key: f.Key, value: f.Value}
		}
		var respErr error
		if p.ErrMsg != "" {
			respErr = fmt.Errorf("%s", p.ErrMsg)
		}
		entries = append(entries, historyEntry{
			epIndex: p.EpIndex,
			fields:  fields,
			response: httpResponseMsg{
				method:     p.Method,
				url:        p.URL,
				statusCode: p.StatusCode,
				body:       p.Body,
				err:        respErr,
				elapsed:    time.Duration(p.ElapsedMs) * time.Millisecond,
			},
			firedAt: p.FiredAt,
		})
	}
	return entries
}

func saveHistory(baseDir string, history []historyEntry) {
	path := historyFilePath(baseDir)
	if path == "" {
		return
	}
	persisted := make([]persistedEntry, len(history))
	for i, e := range history {
		fields := make([]persistedField, len(e.fields))
		for j, f := range e.fields {
			fields[j] = persistedField{Section: f.section, Key: f.key, Value: f.value}
		}
		errMsg := ""
		if e.response.err != nil {
			errMsg = e.response.err.Error()
		}
		persisted[i] = persistedEntry{
			EpIndex:    e.epIndex,
			Fields:     fields,
			Method:     e.response.method,
			URL:        e.response.url,
			StatusCode: e.response.statusCode,
			Body:       e.response.body,
			ErrMsg:     errMsg,
			ElapsedMs:  e.response.elapsed.Milliseconds(),
			FiredAt:    e.firedAt,
		}
	}
	data, _ := json.MarshalIndent(persisted, "", "  ")
	_ = os.WriteFile(path, data, 0644)
}

// ---------------------------------------------------------------------------
// Model
// ---------------------------------------------------------------------------

type model struct {
	cfg     *ruteYaml.Config
	baseDir string
	version string

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

	// Use mode
	useMode       bool
	useFields     []useModeField // editable fields derived from endpoint schema
	useFieldIdx   int            // which field the cursor is on
	useScroll     int            // scroll offset for use mode pane
	useFiring     bool           // HTTP request in flight
	useResponse   *httpResponseMsg
	useRespScroll int // scroll for response body

	// History
	history       []historyEntry
	historyOpen   bool
	historyCursor int
	historyScroll int

	// Variables
	vars        []ruteVar
	varsOpen    bool
	varsCursor  int                // focused row; -1 = scratch row
	varsFocusOn int                // 0 = name input, 1 = value input
	varsInputs  []textinput.Model  // two inputs per row: [name0, val0, name1, val1, ...]
	varsScratch [2]textinput.Model // scratch inputs for the new-var row

	// Command palette
	commandsOpen   bool
	commandsScroll int
	commandsQuery  string
}

func makeVarsInputs(vars []ruteVar, nameW, valW int) []textinput.Model {
	inputs := make([]textinput.Model, len(vars)*2)
	for i, v := range vars {
		ni := textinput.New()
		ni.SetValue(v.Name)
		ni.Width = nameW
		ni.Prompt = ""
		inputs[i*2] = ni

		vi := textinput.New()
		vi.SetValue(v.Value)
		vi.Width = valW
		vi.Prompt = ""
		inputs[i*2+1] = vi
	}
	return inputs
}

func makeScratchInputs(nameW, valW int) [2]textinput.Model {
	ni := textinput.New()
	ni.Width = nameW
	ni.Prompt = ""
	ni.Placeholder = "NAME"

	vi := textinput.New()
	vi.Width = valW
	vi.Prompt = ""
	vi.Placeholder = "value"

	return [2]textinput.Model{ni, vi}
}

func newModel(cfg *ruteYaml.Config, baseDir, binaryVersion string) model {
	items := buildSidebarItems(cfg)
	cursor := 0
	for i, item := range items {
		if !item.isGroup {
			cursor = i
			break
		}
	}
	vars := loadVars(baseDir)
	const nameW, valW = 20, 30
	return model{
		cfg:         cfg,
		baseDir:     baseDir,
		version:     binaryVersion,
		sidebar:     items,
		cursor:      cursor,
		details:     make(map[int]string),
		history:     loadHistory(baseDir),
		vars:        vars,
		varsCursor:  0,
		varsInputs:  makeVarsInputs(vars, nameW, valW),
		varsScratch: makeScratchInputs(nameW, valW),
	}
}

func (m model) Init() tea.Cmd {
	return textinput.Blink
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

// deleteLastWord removes the last whitespace-delimited word from s.
func deleteLastWord(s string) string {
	s = strings.TrimRight(s, " ")
	i := strings.LastIndexAny(s, " \t/.-_")
	if i < 0 {
		return ""
	}
	return s[:i+1]
}

// ---------------------------------------------------------------------------
// Use mode field helpers
// ---------------------------------------------------------------------------

// newField creates a useModeField with its textinput initialised.
func newField(section, key, value string) useModeField {
	inp := textinput.New()
	inp.SetValue(value)
	inp.Prompt = ""

	ki := textinput.New()
	ki.SetValue(key)
	ki.Prompt = ""
	if section == "headers" {
		ki.Placeholder = "header-name"
	}

	return useModeField{
		section:  section,
		key:      key,
		value:    value,
		input:    inp,
		keyInput: ki,
	}
}

// ---------------------------------------------------------------------------
// Suggestions — common HTTP header names and well-known values
// ---------------------------------------------------------------------------

var commonHeaderKeys = []string{
	"Accept",
	"Accept-Encoding",
	"Accept-Language",
	"Authorization",
	"Cache-Control",
	"Connection",
	"Content-Encoding",
	"Content-Length",
	"Content-Type",
	"Cookie",
	"Host",
	"If-Match",
	"If-Modified-Since",
	"If-None-Match",
	"Origin",
	"Pragma",
	"Referer",
	"User-Agent",
	"X-Api-Key",
	"X-Correlation-Id",
	"X-Forwarded-For",
	"X-Request-Id",
}

// headerValueSuggestions returns well-known values for a given header key.
func headerValueSuggestions(key string) []string {
	switch strings.ToLower(key) {
	case "content-type":
		return []string{
			"application/json",
			"application/x-www-form-urlencoded",
			"multipart/form-data",
			"text/plain",
			"text/html",
		}
	case "accept":
		return []string{
			"application/json",
			"text/html",
			"*/*",
		}
	case "authorization":
		return []string{
			"Bearer ",
			"Basic ",
			"ApiKey ",
		}
	case "cache-control":
		return []string{
			"no-cache",
			"no-store",
			"max-age=0",
		}
	case "connection":
		return []string{"keep-alive", "close"}
	case "accept-encoding":
		return []string{"gzip, deflate, br", "gzip", "identity"}
	}
	return nil
}

// filterSuggestions returns items from pool that have prefix (case-insensitive).
func filterSuggestions(pool []string, prefix string) []string {
	if prefix == "" {
		return append([]string{}, pool...)
	}
	lower := strings.ToLower(prefix)
	var out []string
	for _, s := range pool {
		if strings.HasPrefix(strings.ToLower(s), lower) {
			out = append(out, s)
		}
	}
	return out
}

// buildUseFields constructs the list of editable fields from an endpoint's
// params, query, and body schemas. This is a best-effort flat extraction: for
// each schema we try to parse it and enumerate its top-level properties. If
// parsing fails we fall back to a single raw textarea for the section.
func (m *model) buildUseFields(ep ruteYaml.Endpoint) []useModeField {
	var fields []useModeField

	addSection := func(section string, holder *ruteYaml.SchemaHolder) {
		if holder == nil || holder.Schema == "" {
			return
		}
		sr, err := ruteYaml.ParseSchemaRef(holder.Schema)
		if err != nil {
			fields = append(fields, newField(section, "(raw)", ""))
			return
		}
		absBase, _ := filepath.Abs(m.baseDir)
		schema, err := parser.Parse(absBase, sr.File, sr.Export)
		if err != nil || schema == nil {
			fields = append(fields, newField(section, "(raw)", ""))
			return
		}
		if len(schema.Fields) > 0 {
			for _, f := range schema.Fields {
				fields = append(fields, newField(section, f.Name, ""))
			}
		} else if len(schema.Variants) > 0 {
			fields = append(fields, newField(section, "(union)", ""))
		} else {
			fields = append(fields, newField(section, fmt.Sprintf("(%s)", schema.Type), ""))
		}
	}

	addSection("params", ep.Params)

	// If no params schema but path has :param or {param} segments, auto-generate fields
	if ep.Params == nil || ep.Params.Schema == "" {
		for _, seg := range strings.Split(ep.Path, "/") {
			if strings.HasPrefix(seg, ":") {
				fields = append(fields, newField("params", strings.TrimPrefix(seg, ":"), ""))
			} else if strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}") {
				fields = append(fields, newField("params", seg[1:len(seg)-1], ""))
			}
		}
	}

	addSection("query", ep.Query)
	addSection("body", ep.Body)

	// Start with a blank header row — user can add more with 'a'
	fields = append(fields, newField("headers", "", ""))

	return fields
}

// fieldsToBody converts body fields into a JSON object string.
func fieldsToBody(fields []useModeField) string {
	obj := map[string]string{}
	for _, f := range fields {
		if f.section == "body" && f.value != "" {
			obj[f.key] = f.value
		}
	}
	if len(obj) == 0 {
		return ""
	}
	b, _ := json.MarshalIndent(obj, "", "  ")
	return string(b)
}

// buildURL builds a URL substituting path params and appending query string.
func buildURL(baseURL, path string, fields []useModeField) string {
	for _, f := range fields {
		if f.section == "params" {
			value := strings.TrimSpace(f.value)
			if value == "" {
				continue
			}
			path = strings.ReplaceAll(path, ":"+f.key, value)     // :id style
			path = strings.ReplaceAll(path, "{"+f.key+"}", value) // {id} style
		}
	}

	var qparts []string
	for _, f := range fields {
		if f.section == "query" {
			key := strings.TrimSpace(f.key)
			value := strings.TrimSpace(f.value)
			if key == "" || value == "" {
				continue
			}
			qparts = append(qparts, key+"="+value)
		}
	}

	url := strings.TrimRight(baseURL, "/") + path
	if len(qparts) > 0 {
		url += "?" + strings.Join(qparts, "&")
	}
	return url
}

// buildCurlPreview returns a curl command string for the current field values.
func buildCurlPreview(ep ruteYaml.Endpoint, baseURL string, fields []useModeField) string {
	url := buildURL(baseURL, ep.Path, fields)
	parts := []string{"curl", "-s", "-X", ep.Method, fmt.Sprintf("'%s'", url)}

	for _, f := range fields {
		if f.section == "headers" && f.value != "" {
			parts = append(parts, "-H", fmt.Sprintf("'%s: %s'", f.key, f.value))
		}
	}

	body := fieldsToBody(fields)
	if body != "" {
		parts = append(parts, "-H", "'Content-Type: application/json'")
		parts = append(parts, "-d", fmt.Sprintf("'%s'", body))
	}

	return strings.Join(parts, " ")
}

// fireRequest performs the HTTP call and returns a Cmd that delivers the result.
func fireRequest(epIndex int, ep ruteYaml.Endpoint, baseURL string, fields []useModeField) tea.Cmd {
	return func() tea.Msg {
		url := buildURL(baseURL, ep.Path, fields)
		body := fieldsToBody(fields)
		result := executeRequest(ep.Method, url, body, fields)
		result.epIndex = epIndex
		result.method = ep.Method
		result.url = url
		return result
	}
}

func executeRequest(method, rawURL, body string, fields []useModeField) httpResponseMsg {
	resp := doRequest(method, rawURL, body, fields, "")
	parsed, err := url.Parse(rawURL)
	if err != nil || !shouldRetryLoopback(parsed, resp) {
		return resp
	}

	for _, altHost := range alternateLoopbackHosts(parsed.Hostname()) {
		altResp := doRequest(method, rawURL, body, fields, altHost)
		if altResp.err == nil && altResp.statusCode != 404 {
			return altResp
		}
	}

	return resp
}

func doRequest(method, rawURL, body string, fields []useModeField, dialHostOverride string) httpResponseMsg {
	var bodyReader io.Reader
	if body != "" {
		bodyReader = bytes.NewBufferString(body)
	}

	req, err := http.NewRequest(method, rawURL, bodyReader)
	if err != nil {
		return httpResponseMsg{err: err}
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for _, f := range fields {
		if f.section == "headers" {
			key := strings.TrimSpace(f.key)
			value := strings.TrimSpace(f.value)
			if key == "" || value == "" {
				continue
			}
			req.Header.Set(key, value)
		}
	}
	start := time.Now()
	resp, err := requestClientForURL(rawURL, dialHostOverride).Do(req)
	elapsed := time.Since(start)
	if err != nil {
		return httpResponseMsg{err: err, elapsed: elapsed}
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	bodyText := string(raw)

	var pretty bytes.Buffer
	if json.Indent(&pretty, raw, "", "  ") == nil {
		bodyText = pretty.String()
	}

	return httpResponseMsg{
		statusCode: resp.StatusCode,
		body:       bodyText,
		elapsed:    elapsed,
	}
}

func requestClientForURL(rawURL, dialHostOverride string) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = func(req *http.Request) (*url.URL, error) {
		host := req.URL.Hostname()
		if isLoopbackHost(host) {
			return nil, nil
		}
		return http.ProxyFromEnvironment(req)
	}
	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		if dialHostOverride != "" {
			_, port, err := net.SplitHostPort(addr)
			if err == nil {
				addr = net.JoinHostPort(dialHostOverride, port)
			}
		}
		var d net.Dialer
		return d.DialContext(ctx, network, addr)
	}
	return &http.Client{Transport: transport}
}

func shouldRetryLoopback(parsed *url.URL, resp httpResponseMsg) bool {
	if parsed == nil || !strings.EqualFold(parsed.Hostname(), "localhost") {
		return false
	}
	if resp.err != nil || resp.statusCode != 404 {
		return false
	}
	body := strings.TrimSpace(resp.body)
	return body == "Not Found" || strings.EqualFold(body, "GET not found")
}

func alternateLoopbackHosts(host string) []string {
	if !strings.EqualFold(host, "localhost") {
		return nil
	}
	return []string{"127.0.0.1", "::1"}
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func copyToClipboardCmd(text string) tea.Cmd {
	return func() tea.Msg {
		candidates := [][]string{
			{"pbcopy"},
			{"wl-copy"},
			{"xclip", "-selection", "clipboard"},
			{"clip.exe"},
		}

		for _, candidate := range candidates {
			if runtime.GOOS == "darwin" && candidate[0] != "pbcopy" {
				continue
			}
			if runtime.GOOS == "windows" && candidate[0] != "clip.exe" {
				continue
			}

			cmd := exec.Command(candidate[0], candidate[1:]...)
			cmd.Stdin = strings.NewReader(text)
			if err := cmd.Run(); err == nil {
				return clipboardMsg{}
			}
		}

		return clipboardMsg{err: fmt.Errorf("clipboard tool not available")}
	}
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
		// ctrl+p toggles command palette from anywhere (except when typing in a field)
		if msg.String() == "ctrl+p" {
			editing := m.useMode && m.useFieldIdx >= 0 && m.useFieldIdx < len(m.useFields) &&
				(m.useFields[m.useFieldIdx].editing || m.useFields[m.useFieldIdx].editingKey)
			if !editing {
				m.commandsOpen = !m.commandsOpen
				m.commandsScroll = 0
				m.commandsQuery = ""
				return m, nil
			}
		}
		if m.commandsOpen {
			switch msg.String() {
			case "esc", "ctrl+p":
				m.commandsOpen = false
				m.commandsQuery = ""
			case "up", "k":
				if m.commandsScroll > 0 {
					m.commandsScroll--
				}
			case "down", "j":
				m.commandsScroll++
			case "backspace":
				if len(m.commandsQuery) > 0 {
					m.commandsQuery = m.commandsQuery[:len(m.commandsQuery)-1]
					m.commandsScroll = 0
				}
			default:
				if len(msg.Runes) > 0 {
					m.commandsQuery += string(msg.Runes)
					m.commandsScroll = 0
				}
			}
			return m, nil
		}
		if m.varsOpen {
			return m.updateVarsModal(msg)
		}
		if m.historyOpen {
			return m.updateHistoryModal(msg)
		}
		if m.searching {
			return m.updateSearchMode(msg)
		}
		if m.useMode {
			return m.updateUseMode(msg)
		}
		return m.updateNormal(msg)
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case httpResponseMsg:
		m.useFiring = false
		m.useResponse = &msg
		m.useRespScroll = 0
		// Save to history regardless of success/failure
		m.history = append(m.history, historyEntry{
			epIndex:  msg.epIndex,
			fields:   append([]useModeField{}, m.useFields...),
			response: msg,
			firedAt:  time.Now(),
		})
		m.historyCursor = 0
		saveHistory(m.baseDir, m.history)
	case clipboardMsg:
	default:
		// Forward tick messages to whichever textinput is focused so the cursor blinks
		var cmd tea.Cmd
		if m.useMode && m.useFieldIdx >= 0 && m.useFieldIdx < len(m.useFields) {
			f := &m.useFields[m.useFieldIdx]
			if f.editing {
				f.input, cmd = f.input.Update(msg)
				return m, cmd
			}
			if f.editingKey {
				f.keyInput, cmd = f.keyInput.Update(msg)
				return m, cmd
			}
		}
		if m.varsOpen {
			if inp := m.activeVarsInput(); inp != nil && inp.Focused() {
				*inp, cmd = inp.Update(msg)
				return m, cmd
			}
		}
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
	case "enter":
		ep := m.cfg.Endpoints[m.selectedEndpointIndex()]
		m.useMode = true
		m.useFields = m.buildUseFields(ep)
		m.useFieldIdx = 0
		m.useResponse = nil
		m.useFiring = false
		m.useScroll = 0
		m.useRespScroll = 0
	case "h":
		if len(m.history) > 0 {
			m.historyOpen = true
			m.historyCursor = 0
			m.historyScroll = 0
		}
	case "v":
		m.varsOpen = true
		m.varsCursor = 0
		m.varsFocusOn = 0
		m.blurAllVarsInputs()
	}
	return m, nil
}

func (m model) updateSearchMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
		if len(msg.Runes) > 0 {
			m.searchQuery += string(msg.Runes)
			m.updateSearch()
		}
	}
	return m, nil
}

func (m model) updateUseMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Route input through textinput when a field is focused
	if m.useFieldIdx >= 0 && m.useFieldIdx < len(m.useFields) {
		f := &m.useFields[m.useFieldIdx]
		if f.editingKey {
			switch msg.String() {
			case "esc":
				f.editingKey = false
				f.keyInput.Blur()
				f.showSuggestion = false
			case "down":
				// Navigate suggestion list
				if f.showSuggestion && len(f.suggestions) > 0 {
					f.suggestionIdx++
					if f.suggestionIdx >= len(f.suggestions) {
						f.suggestionIdx = 0
					}
				}
			case "up":
				if f.showSuggestion && len(f.suggestions) > 0 {
					f.suggestionIdx--
					if f.suggestionIdx < 0 {
						f.suggestionIdx = len(f.suggestions) - 1
					}
				}
			case "tab", "enter":
				// Accept suggestion if one is highlighted, else commit key
				if f.showSuggestion && f.suggestionIdx >= 0 && f.suggestionIdx < len(f.suggestions) {
					chosen := f.suggestions[f.suggestionIdx]
					f.key = chosen
					f.keyInput.SetValue(chosen)
					f.showSuggestion = false
					f.suggestionIdx = -1
					// Move to value editing
					f.editingKey = false
					f.keyInput.Blur()
					f.editing = true
					f.input.Focus()
					// Seed value suggestions
					f.suggestions = headerValueSuggestions(chosen)
					f.showSuggestion = len(f.suggestions) > 0
					f.suggestionIdx = -1
				} else {
					f.editingKey = false
					f.keyInput.Blur()
					f.key = f.keyInput.Value()
					f.editing = true
					f.input.Focus()
					f.suggestions = headerValueSuggestions(f.key)
					f.showSuggestion = len(f.suggestions) > 0
					f.suggestionIdx = -1
				}
			default:
				var cmd tea.Cmd
				f.keyInput, cmd = f.keyInput.Update(msg)
				f.key = f.keyInput.Value()
				// Recompute suggestions
				f.suggestions = filterSuggestions(commonHeaderKeys, f.key)
				f.showSuggestion = len(f.suggestions) > 0
				f.suggestionIdx = -1
				return m, cmd
			}
			return m, nil
		}
		if f.editing {
			switch msg.String() {
			case "esc", "enter":
				f.editing = false
				f.input.Blur()
				f.value = f.input.Value()
				f.showSuggestion = false
			case "down":
				if f.showSuggestion && len(f.suggestions) > 0 {
					f.suggestionIdx++
					if f.suggestionIdx >= len(f.suggestions) {
						f.suggestionIdx = 0
					}
				} else {
					// pass through — no suggestion open, do nothing special
				}
			case "up":
				if f.showSuggestion && len(f.suggestions) > 0 {
					f.suggestionIdx--
					if f.suggestionIdx < 0 {
						f.suggestionIdx = len(f.suggestions) - 1
					}
				}
			case "tab":
				// Accept suggestion if one is selected
				if f.showSuggestion && f.suggestionIdx >= 0 && f.suggestionIdx < len(f.suggestions) {
					f.value = f.suggestions[f.suggestionIdx]
					f.input.SetValue(f.value)
					f.input.CursorEnd()
					f.showSuggestion = false
					f.suggestionIdx = -1
				} else {
					f.editing = false
					f.input.Blur()
					f.value = f.input.Value()
					f.showSuggestion = false
					next := m.useFieldIdx + 1
					if next < len(m.useFields) {
						m.useFieldIdx = next
						nf := &m.useFields[m.useFieldIdx]
						if nf.section == "headers" {
							nf.editingKey = true
							nf.keyInput.Focus()
							nf.suggestions = filterSuggestions(commonHeaderKeys, nf.key)
							nf.showSuggestion = len(nf.suggestions) > 0
							nf.suggestionIdx = -1
						} else {
							nf.editing = true
							nf.input.Focus()
						}
					}
				}
			default:
				var cmd tea.Cmd
				f.input, cmd = f.input.Update(msg)
				f.value = f.input.Value()
				// Value suggestions only for header fields
				if f.section == "headers" {
					f.suggestions = filterSuggestions(headerValueSuggestions(f.key), f.value)
					f.showSuggestion = len(f.suggestions) > 0
					f.suggestionIdx = -1
				}
				return m, cmd
			}
			return m, nil
		}
	}

	switch msg.String() {
	case "esc", "q":
		m.useMode = false
		m.useResponse = nil
		m.useFiring = false
	case "up", "k":
		if m.useFieldIdx > 0 {
			m.useFieldIdx--
		} else if m.useRespScroll > 0 && m.useResponse != nil {
			m.useRespScroll--
		}
	case "down", "j":
		if m.useFieldIdx < len(m.useFields)-1 {
			m.useFieldIdx++
		} else if m.useResponse != nil {
			m.useRespScroll++
		}
	case "enter", " ":
		if m.useFieldIdx >= 0 && m.useFieldIdx < len(m.useFields) {
			f := &m.useFields[m.useFieldIdx]
			if f.section == "headers" && f.key == "" {
				f.editingKey = true
				f.keyInput.Focus()
				f.suggestions = filterSuggestions(commonHeaderKeys, "")
				f.showSuggestion = len(f.suggestions) > 0
				f.suggestionIdx = -1
			} else if f.section == "headers" {
				f.editing = true
				f.input.SetValue(f.value)
				f.input.Focus()
				f.input.CursorEnd()
				f.suggestions = filterSuggestions(headerValueSuggestions(f.key), f.value)
				f.showSuggestion = len(f.suggestions) > 0
				f.suggestionIdx = -1
			} else {
				f.editing = true
				f.input.SetValue(f.value)
				f.input.Focus()
				f.input.CursorEnd()
				f.showSuggestion = false
			}
		}
	case "a":
		// Only add a header row when cursor is on the headers section
		onHeaders := m.useFieldIdx >= 0 && m.useFieldIdx < len(m.useFields) &&
			m.useFields[m.useFieldIdx].section == "headers"
		if onHeaders || m.useFieldIdx >= len(m.useFields) {
			insertAt := m.useFieldIdx + 1
			if insertAt > len(m.useFields) {
				insertAt = len(m.useFields)
			}
			nf := newField("headers", "", "")
			m.useFields = append(m.useFields[:insertAt], append([]useModeField{nf}, m.useFields[insertAt:]...)...)
			m.useFieldIdx = insertAt
			m.useFields[insertAt].editingKey = true
			m.useFields[insertAt].keyInput.Focus()
			m.useFields[insertAt].suggestions = filterSuggestions(commonHeaderKeys, "")
			m.useFields[insertAt].showSuggestion = true
			m.useFields[insertAt].suggestionIdx = -1
		}
	case "ctrl+x":
		// Delete focused field if it's a header
		if m.useFieldIdx >= 0 && m.useFieldIdx < len(m.useFields) {
			if m.useFields[m.useFieldIdx].section == "headers" {
				m.useFields = append(m.useFields[:m.useFieldIdx], m.useFields[m.useFieldIdx+1:]...)
				if m.useFieldIdx >= len(m.useFields) {
					m.useFieldIdx = len(m.useFields) - 1
				}
			}
		}
	case "ctrl+r", "f5":
		if !m.useFiring {
			m.useFiring = true
			m.useResponse = nil
			epIdx := m.selectedEndpointIndex()
			ep := m.cfg.Endpoints[epIdx]
			resolved := resolveFields(m.useFields, m.vars)
			return m, fireRequest(epIdx, ep, m.cfg.BaseURL, resolved)
		}
	case "y":
		ep := m.cfg.Endpoints[m.selectedEndpointIndex()]
		resolved := resolveFields(m.useFields, m.vars)
		return m, copyToClipboardCmd(buildCurlPreview(ep, m.cfg.BaseURL, resolved))
	case "h":
		if len(m.history) > 0 {
			m.historyOpen = true
			m.historyCursor = 0
			m.historyScroll = 0
		}
	case "v":
		m.varsOpen = true
		m.varsFocusOn = 0
		m.blurAllVarsInputs()
		// If focused field contains {{VAR}}, jump to or pre-fill that var
		varName := ""
		if m.useFieldIdx >= 0 && m.useFieldIdx < len(m.useFields) {
			val := m.useFields[m.useFieldIdx].value
			if start := strings.Index(val, "{{"); start >= 0 {
				if end := strings.Index(val, "}}"); end > start {
					varName = val[start+2 : end]
				}
			}
		}
		if varName != "" {
			found := false
			for i, v := range m.vars {
				if v.Name == varName {
					m.varsCursor = i
					found = true
					break
				}
			}
			if !found {
				// Pre-fill scratch with the var name, drop straight into value
				m.varsCursor = -1
				m.varsScratch = makeScratchInputs(varsNameW, varsValW)
				m.varsScratch[0].SetValue(varName)
				m.varsScratch[1].Focus()
				m.varsFocusOn = 1
			}
		} else {
			m.varsCursor = 0
		}
	case "pgup", "ctrl+u":
		m.useRespScroll -= m.height / 4
		if m.useRespScroll < 0 {
			m.useRespScroll = 0
		}
	case "pgdown":
		if m.useResponse != nil {
			m.useRespScroll += m.height / 4
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
// Variables modal
// ---------------------------------------------------------------------------

const varsNameW = 20
const varsValW = 30

// activeVarsInput returns a pointer to the currently focused textinput, or nil.
func (m *model) activeVarsInput() *textinput.Model {
	if m.varsCursor == -1 {
		return &m.varsScratch[m.varsFocusOn]
	}
	if m.varsCursor >= 0 && m.varsCursor < len(m.vars) {
		idx := m.varsCursor*2 + m.varsFocusOn
		if idx < len(m.varsInputs) {
			return &m.varsInputs[idx]
		}
	}
	return nil
}

func (m model) updateVarsModal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	inp := m.activeVarsInput()
	focused := inp != nil && inp.Focused()

	if focused {
		switch msg.String() {
		case "esc":
			inp.Blur()
			m.syncVarsFromInputs()
		case "tab":
			inp.Blur()
			m.syncVarsFromInputs()
			if m.varsFocusOn == 0 {
				// name → value
				m.varsFocusOn = 1
			} else {
				// value → commit + move to next row name
				m.varsFocusOn = 0
				if m.varsCursor == -1 {
					// commit scratch
					name := strings.TrimSpace(m.varsScratch[0].Value())
					val := m.varsScratch[1].Value()
					if name != "" {
						m.vars = append(m.vars, ruteVar{Name: name, Value: val})
						m.varsInputs = makeVarsInputs(m.vars, varsNameW, varsValW)
						saveVars(m.baseDir, m.vars)
						m.varsCursor = len(m.vars) - 1
					}
					m.varsScratch = makeScratchInputs(varsNameW, varsValW)
				} else if m.varsCursor < len(m.vars)-1 {
					m.varsCursor++
				}
			}
			// Focus the new active input
			if ni := m.activeVarsInput(); ni != nil {
				ni.Focus()
			}
		case "enter":
			inp.Blur()
			m.syncVarsFromInputs()
			if m.varsCursor == -1 && m.varsFocusOn == 1 {
				name := strings.TrimSpace(m.varsScratch[0].Value())
				val := m.varsScratch[1].Value()
				if name != "" {
					m.vars = append(m.vars, ruteVar{Name: name, Value: val})
					m.varsInputs = makeVarsInputs(m.vars, varsNameW, varsValW)
					saveVars(m.baseDir, m.vars)
					m.varsCursor = len(m.vars) - 1
				}
				m.varsScratch = makeScratchInputs(varsNameW, varsValW)
			}
		default:
			var cmd tea.Cmd
			*inp, cmd = inp.Update(msg)
			return m, cmd
		}
		return m, nil
	}

	// Navigation (no input focused)
	switch msg.String() {
	case "esc", "v":
		m.varsOpen = false
		m.blurAllVarsInputs()
	case "up", "k":
		if m.varsCursor > 0 {
			m.varsCursor--
			m.varsFocusOn = 0
		}
	case "down", "j":
		if m.varsCursor < len(m.vars)-1 {
			m.varsCursor++
			m.varsFocusOn = 0
		}
	case "enter", " ":
		m.varsFocusOn = 0
		if ni := m.activeVarsInput(); ni != nil {
			ni.Focus()
		}
	case "n":
		m.varsCursor = -1
		m.varsFocusOn = 0
		m.varsScratch = makeScratchInputs(varsNameW, varsValW)
		m.varsScratch[0].Focus()
	case "ctrl+x":
		if m.varsCursor >= 0 && m.varsCursor < len(m.vars) {
			m.vars = append(m.vars[:m.varsCursor], m.vars[m.varsCursor+1:]...)
			m.varsInputs = makeVarsInputs(m.vars, varsNameW, varsValW)
			saveVars(m.baseDir, m.vars)
			if m.varsCursor >= len(m.vars) {
				m.varsCursor = len(m.vars) - 1
			}
		}
	}
	return m, nil
}

// syncVarsFromInputs writes textinput values back to m.vars and saves.
func (m *model) syncVarsFromInputs() {
	for i := range m.vars {
		if i*2+1 < len(m.varsInputs) {
			m.vars[i].Name = m.varsInputs[i*2].Value()
			m.vars[i].Value = m.varsInputs[i*2+1].Value()
		}
	}
	saveVars(m.baseDir, m.vars)
}

// blurAllVarsInputs blurs every textinput in the vars modal.
func (m *model) blurAllVarsInputs() {
	for i := range m.varsInputs {
		m.varsInputs[i].Blur()
	}
	m.varsScratch[0].Blur()
	m.varsScratch[1].Blur()
}

func (m model) renderVarsModal(width, height int) string {
	modalW := width * 60 / 100
	if modalW < 58 {
		modalW = 58
	}
	if modalW > 100 {
		modalW = 100
	}
	modalH := 20
	if modalH > height-4 {
		modalH = height - 4
	}

	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	active := lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true)
	yellow := lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true)

	var rows []string

	rows = append(rows, titleLook.Render("Variables")+"  "+helpLook.Render("{{NAME}} in any field  •  n new  •  ctrl+x delete  •  esc close"))
	rows = append(rows, "")

	// Column headers
	hdrStyle := dim.Copy().Bold(true)
	nameHdr := hdrStyle.Width(varsNameW + 2).Render("NAME")
	valHdr := hdrStyle.Width(varsValW + 2).Render("VALUE")
	rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, nameHdr, "   ", valHdr))
	rows = append(rows, dim.Render(strings.Repeat("─", varsNameW+varsValW+8)))

	renderVarRow := func(idx int, nameInput, valInput textinput.Model, isCursor bool) string {
		nameFocused := nameInput.Focused()
		valFocused := valInput.Focused()

		var prefix string
		switch {
		case nameFocused:
			prefix = yellow.Render("▸ ")
		case valFocused:
			prefix = yellow.Render("  ")
		case isCursor:
			prefix = active.Render("▸ ")
		default:
			prefix = "  "
		}

		_ = idx
		return prefix + nameInput.View() + dim.Render("  =  ") + valInput.View()
	}

	for i, v := range m.vars {
		_ = v
		isCursor := i == m.varsCursor
		ni := m.varsInputs[i*2]
		vi := m.varsInputs[i*2+1]
		rows = append(rows, renderVarRow(i, ni, vi, isCursor))
	}

	if len(m.vars) == 0 {
		rows = append(rows, dim.Render("  No variables yet. Press n to add one."))
	}

	rows = append(rows, "")

	// Scratch row
	if m.varsCursor == -1 {
		rows = append(rows, renderVarRow(-1, m.varsScratch[0], m.varsScratch[1], true))
	} else {
		rows = append(rows, dim.Render("  n  new variable"))
	}

	// Clamp to visible height
	visibleH := modalH - 4
	if len(rows) > visibleH {
		rows = rows[:visibleH]
	}

	content := strings.Join(rows, "\n")

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("12")).
		Padding(1, 2).
		Width(modalW)

	modal := box.Render(content)

	leftPad := (width - lipgloss.Width(modal)) / 2
	topPad := (height - lipgloss.Height(modal)) / 2
	if leftPad < 0 {
		leftPad = 0
	}
	if topPad < 0 {
		topPad = 0
	}

	var sb strings.Builder
	blank := strings.Repeat(" ", width)
	for i := 0; i < topPad; i++ {
		sb.WriteString(blank + "\n")
	}
	pad := strings.Repeat(" ", leftPad)
	for _, line := range strings.Split(modal, "\n") {
		sb.WriteString(pad + line + "\n")
	}
	return sb.String()
}

// ---------------------------------------------------------------------------
// History modal
// ---------------------------------------------------------------------------

func (m model) updateHistoryModal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q", "h":
		m.historyOpen = false
	case "up", "k":
		// up = towards newer (smaller display index)
		if m.historyCursor > 0 {
			m.historyCursor--
		}
	case "down", "j":
		// down = towards older (larger display index)
		if m.historyCursor < len(m.history)-1 {
			m.historyCursor++
		}
	case "enter":
		// displayIdx 0 = newest = history[len-1]
		histIdx := len(m.history) - 1 - m.historyCursor
		if histIdx < 0 || histIdx >= len(m.history) {
			break
		}
		entry := m.history[histIdx]
		for i, item := range m.sidebar {
			if !item.isGroup && item.epIndex == entry.epIndex {
				m.cursor = i
				break
			}
		}
		m.useMode = true
		// Rebuild fields from history with fresh textinputs
		restored := append([]useModeField{}, entry.fields...)
		for i, f := range restored {
			nf := newField(f.section, f.key, f.value)
			restored[i] = nf
		}
		m.useFields = restored
		m.useFieldIdx = 0
		m.useResponse = &entry.response
		m.useFiring = false
		m.useScroll = 0
		m.useRespScroll = 0
		m.historyOpen = false
	case "pgup", "ctrl+u":
		m.historyScroll -= m.height / 4
		if m.historyScroll < 0 {
			m.historyScroll = 0
		}
	case "pgdown", "ctrl+d":
		m.historyScroll += m.height / 4
	}
	return m, nil
}

// ---------------------------------------------------------------------------
// Command palette
// ---------------------------------------------------------------------------

type cmdEntry struct {
	keys    string
	desc    string
	context string // "global", "browse", "use", "search", "history"
}

var allCommands = []cmdEntry{
	// Global
	{"ctrl+p", "Open command palette", "global"},
	{"q / ctrl+c", "Quit", "global"},
	// Browse
	{"↑ / k", "Move up", "browse"},
	{"↓ / j", "Move down", "browse"},
	{"pgup / ctrl+u", "Scroll detail up", "browse"},
	{"pgdn / ctrl+d", "Scroll detail down", "browse"},
	{"/", "Search endpoints", "browse"},
	{"enter", "Open endpoint in use mode", "browse"},
	{"h", "Open request history", "browse"},
	{"v", "Open variables", "browse"},
	// Search
	{"↑ / ctrl+p", "Previous result", "search"},
	{"↓ / ctrl+n", "Next result", "search"},
	{"enter", "Jump to result", "search"},
	{"esc", "Close search", "search"},
	// Use mode
	{"↑ / k", "Previous field", "use"},
	{"↓ / j", "Next field", "use"},
	{"enter / space", "Edit focused field", "use"},
	{"tab", "Confirm and move to next field", "use"},
	{"esc", "Confirm and stop editing", "use"},
	{"ctrl+u", "Clear field", "use"},
	{"ctrl+w", "Delete last word", "use"},
	{"ctrl+r / f5", "Fire request", "use"},
	{"y", "Copy curl command", "use"},
	{"a", "Add header row (when on headers)", "use"},
	{"ctrl+x", "Delete header row", "use"},
	{"h", "Open request history", "use"},
	{"v", "Open variables", "use"},
	{"esc (no edit)", "Back to browse", "use"},
	// History
	{"↑ / k", "Previous entry", "history"},
	{"↓ / j", "Next entry", "history"},
	{"enter", "Restore entry into use mode", "history"},
	{"esc / h", "Close history", "history"},
	// Variables
	{"↑ / k", "Previous variable", "vars"},
	{"↓ / j", "Next variable", "vars"},
	{"enter / space", "Edit name of focused var", "vars"},
	{"tab", "Move name → value → next", "vars"},
	{"n", "New variable", "vars"},
	{"ctrl+x", "Delete focused variable", "vars"},
	{"esc / v", "Close variables", "vars"},
}

func (m model) renderCommandsModal(width, height int) string {
	modalW := width * 60 / 100
	if modalW < 58 {
		modalW = 58
	}
	if modalW > 100 {
		modalW = 100
	}

	// Fixed compact height — content scrolls inside it
	modalH := 20
	if modalH > height-4 {
		modalH = height - 4
	}

	innerW := modalW - 6
	keyW := 22
	descW := innerW - keyW - 2
	if descW < 10 {
		descW = 10
	}

	// Filter commands by query
	q := strings.ToLower(m.commandsQuery)
	filtered := allCommands
	if q != "" {
		filtered = nil
		for _, cmd := range allCommands {
			if strings.Contains(strings.ToLower(cmd.keys), q) ||
				strings.Contains(strings.ToLower(cmd.desc), q) ||
				strings.Contains(strings.ToLower(cmd.context), q) {
				filtered = append(filtered, cmd)
			}
		}
	}

	// Build content lines
	var all []string
	if q == "" {
		// Grouped view
		contexts := []struct {
			id    string
			label string
		}{
			{"global", "Global"},
			{"browse", "Browse"},
			{"search", "Search"},
			{"use", "Use mode"},
			{"history", "History"},
			{"vars", "Variables"},
		}
		for _, ctx := range contexts {
			hasAny := false
			for _, cmd := range filtered {
				if cmd.context == ctx.id {
					hasAny = true
					break
				}
			}
			if !hasAny {
				continue
			}
			all = append(all, useSectionStyle.Render(ctx.label))
			for _, cmd := range filtered {
				if cmd.context != ctx.id {
					continue
				}
				keyPart := lipgloss.NewStyle().
					Foreground(lipgloss.Color("11")).Bold(true).Width(keyW).Render(cmd.keys)
				descPart := helpLook.Width(descW).Render(cmd.desc)
				all = append(all, "  "+lipgloss.JoinHorizontal(lipgloss.Top, keyPart, descPart))
			}
			all = append(all, "")
		}
		// Trim trailing blank
		for len(all) > 0 && all[len(all)-1] == "" {
			all = all[:len(all)-1]
		}
	} else {
		// Flat filtered view
		if len(filtered) == 0 {
			all = append(all, searchDimStyle.Render("  No matches"))
		}
		for _, cmd := range filtered {
			keyPart := lipgloss.NewStyle().
				Foreground(lipgloss.Color("11")).Bold(true).Width(keyW).Render(cmd.keys)
			ctxTag := helpLook.Render("[" + cmd.context + "]  ")
			descPart := helpLook.Width(descW).Render(cmd.desc)
			all = append(all, "  "+lipgloss.JoinHorizontal(lipgloss.Top, keyPart, ctxTag, descPart))
		}
	}

	// Apply scroll
	totalLines := len(all)
	visibleH := modalH - 6 // border + padding + header + search input + spacer

	scroll := m.commandsScroll
	if scroll > totalLines-visibleH {
		scroll = totalLines - visibleH
	}
	if scroll < 0 {
		scroll = 0
	}
	visible := all
	if scroll < len(visible) {
		visible = visible[scroll:]
	}
	if len(visible) > visibleH {
		visible = visible[:visibleH]
	}

	// Scroll hint
	scrollHint := ""
	canScrollDown := scroll+visibleH < totalLines
	canScrollUp := scroll > 0
	switch {
	case canScrollUp && canScrollDown:
		scrollHint = "  " + helpLook.Render("↑↓ scroll")
	case canScrollUp:
		scrollHint = "  " + helpLook.Render("↑ scroll up")
	case canScrollDown:
		scrollHint = "  " + helpLook.Render("↓ scroll down")
	}

	// Search input line
	searchPrompt := searchPromptStyle.Render("/ ")
	searchText := searchInputStyle.Render(m.commandsQuery)
	searchCursor := searchPromptStyle.Render("▋")

	header := titleLook.Render("Commands") + "  " + helpLook.Render("esc close") + scrollHint
	searchLine := searchPrompt + searchText + searchCursor

	content := header + "\n" + searchLine + "\n\n" + strings.Join(visible, "\n")

	modalStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("12")).
		Padding(1, 2).
		Width(modalW).
		Height(modalH)

	modal := modalStyle.Render(content)

	leftPad := (width - modalW) / 2
	topPad := (height - modalH) / 2
	if leftPad < 0 {
		leftPad = 0
	}
	if topPad < 0 {
		topPad = 0
	}

	var sb strings.Builder
	blank := strings.Repeat(" ", width)
	for i := 0; i < topPad; i++ {
		sb.WriteString(blank + "\n")
	}
	pad := strings.Repeat(" ", leftPad)
	for _, ml := range strings.Split(modal, "\n") {
		sb.WriteString(pad + ml + "\n")
	}
	return sb.String()
}

func (m model) renderHistoryModal(width, height int) string {
	modalW := width * 70 / 100
	if modalW < 60 {
		modalW = 60
	}
	if modalW > 120 {
		modalW = 120
	}
	modalH := height * 75 / 100
	if modalH < 10 {
		modalH = 10
	}

	innerW := modalW - 4
	var lines []string

	lines = append(lines, titleLook.Render("Request History"))
	lines = append(lines, helpLook.Render("↑/↓ navigate  •  enter restore  •  esc close"))
	lines = append(lines, dividerStyle.Render(strings.Repeat("─", innerW)))
	lines = append(lines, "")

	if len(m.history) == 0 {
		lines = append(lines, helpLook.Render("No requests fired yet."))
	}

	for i := len(m.history) - 1; i >= 0; i-- {
		entry := m.history[i]
		ep := m.cfg.Endpoints[entry.epIndex]

		timestamp := entry.firedAt.Format("15:04:05")

		var statusPart string
		if entry.response.err != nil {
			statusPart = respErrStyle.Render("ERR")
		} else {
			code := entry.response.statusCode
			s := fmt.Sprintf("%d", code)
			if code >= 400 {
				statusPart = respErrStyle.Render(s)
			} else {
				statusPart = respOKStyle.Render(s)
			}
		}

		elapsed := fmt.Sprintf("%dms", entry.response.elapsed.Milliseconds())
		method := renderer.MethodStyle(fmt.Sprintf("%-6s", ep.Method))
		path := ep.Path

		// displayIdx 0 = newest (top of list)
		displayIdx := len(m.history) - 1 - i
		isSelected := displayIdx == m.historyCursor

		timeStr := helpLook.Render(timestamp + "  ")
		elapsedStr := helpLook.Render("  " + elapsed)
		pathStr := path
		if isSelected {
			pathStr = selectedBg.Render(path)
		}
		row := lipgloss.JoinHorizontal(lipgloss.Center,
			timeStr,
			method, " ",
			pathStr,
			"  ", statusPart,
			elapsedStr,
		)
		lines = append(lines, row)
	}

	// Apply scroll
	if m.historyScroll > len(lines)-1 {
		m.historyScroll = len(lines) - 1
	}
	visible := lines
	if m.historyScroll < len(visible) {
		visible = visible[m.historyScroll:]
	}
	if len(visible) > modalH-2 {
		visible = visible[:modalH-2]
	}

	content := strings.Join(visible, "\n")

	modalStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("12")).
		Padding(1, 2).
		Width(modalW).
		Height(modalH)

	modal := modalStyle.Render(content)

	// Centre the modal in the terminal
	leftPad := (width - modalW) / 2
	topPad := (height - modalH) / 2
	if leftPad < 0 {
		leftPad = 0
	}
	if topPad < 0 {
		topPad = 0
	}

	var sb strings.Builder
	blank := strings.Repeat(" ", width)
	for i := 0; i < topPad; i++ {
		sb.WriteString(blank + "\n")
	}
	modalLines := strings.Split(modal, "\n")
	pad := strings.Repeat(" ", leftPad)
	for _, ml := range modalLines {
		sb.WriteString(pad + ml + "\n")
	}
	return sb.String()
}

// ---------------------------------------------------------------------------
// Styles
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

	// Use mode specific styles
	useLabelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8")).
			Width(14)

	usePlaceholderStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("8")).
				Italic(true)

	useKeyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("7"))

	// Use mode rows
	useFieldStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("7")).
			Background(lipgloss.Color("236")).
			Padding(0, 1)

	useFieldActiveStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("15")).
				Background(lipgloss.Color("239")).
				Padding(0, 1)

	useFieldEditingStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("15")).
				Background(lipgloss.Color("12")).
				Padding(0, 1)

	useSectionStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8")).
			Bold(true).
			MarginTop(1)

	curlPreviewStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("8")).
				Padding(0, 1)

	respHeaderStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15"))

	respOKStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("10"))

	respErrStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("9"))

	respBodyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("7"))

	firingStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("11")).
			Bold(true)

	dividerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8"))

	fireHintStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15")).
			Background(lipgloss.Color("2")).
			Padding(0, 1)

	// Suggestion dropdown styles
	suggestionNormalStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("7")).
				Padding(0, 1)

	suggestionActiveStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("15")).
				Background(lipgloss.Color("4")).
				Padding(0, 1)
)

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

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

	var detail string
	if m.useMode {
		detail = m.renderUseMode(detailWidth, m.height-2)
	} else {
		detail = m.renderDetail(detailWidth, m.height-2)
	}

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
	} else if m.useMode {
		help = helpLook.Render("  ctrl+r send  •  y copy curl  •  esc leave  •  ctrl+p commands")
	} else {
		help = helpLook.Render("  ctrl+p  commands")
	}

	base := main + "\n" + help

	if m.commandsOpen {
		return m.renderCommandsModal(m.width, m.height-1) + "\n" + help
	}
	if m.varsOpen {
		return m.renderVarsModal(m.width, m.height-1) + "\n" + help
	}
	if m.historyOpen {
		return m.renderHistoryModal(m.width, m.height-1) + "\n" + help
	}

	return base
}

// ---------------------------------------------------------------------------
// Use mode renderer
// ---------------------------------------------------------------------------

func (m model) renderUseMode(width, height int) string {
	selected := m.selectedEndpointIndex()
	if selected < 0 || selected >= len(m.cfg.Endpoints) {
		return detailPad.Width(width).Render("No endpoint selected.")
	}
	ep := m.cfg.Endpoints[selected]

	// Content is built as individual segments; each segment is one or more lines.
	// We'll join them, apply scroll, then clip to height.
	var lines []string

	// ── Endpoint heading ────────────────────────────────────────────────────
	heading := lipgloss.NewStyle().Bold(true)
	lines = append(lines,
		renderer.MethodStyle(ep.Method)+" "+heading.Render(ep.Path),
	)
	if ep.Description != "" {
		lines = append(lines, helpLook.Render(ep.Description))
	}
	lines = append(lines, "")

	// ── Fields ───────────────────────────────────────────────────────────────
	// Layout: compact label column + value column.
	labelW := 14
	rowW := width - labelW - 3
	if rowW < 10 {
		rowW = 10
	}
	// Headers get a compact key and a wider value area so they stay on one row.
	hKeyW := rowW / 3
	if hKeyW < 12 {
		hKeyW = 12
	}
	if hKeyW > 18 {
		hKeyW = 18
	}
	hValW := rowW - hKeyW - 1
	if hValW < 10 {
		hValW = 10
	}

	currentSection := ""
	activeSuggestions := []string(nil)
	activeSuggestionIdx := -1

	for i, f := range m.useFields {
		// Section heading
		if f.section != currentSection {
			currentSection = f.section
			label := strings.ToUpper(currentSection[:1]) + currentSection[1:]
			lines = append(lines, useSectionStyle.Render(label))
		}

		isActive := i == m.useFieldIdx

		// Collect active field's suggestion state for rendering after this row
		if isActive && f.showSuggestion {
			activeSuggestions = f.suggestions
			activeSuggestionIdx = f.suggestionIdx
		}

		if f.section == "headers" {
			keyDisplay := f.key
			if keyDisplay == "" {
				keyDisplay = usePlaceholderStyle.Render("header-name")
			} else {
				keyDisplay = useKeyStyle.Render(keyDisplay)
			}
			valDisplay := f.value
			if valDisplay == "" {
				valDisplay = usePlaceholderStyle.Render("value")
			}

			var keyPart, valPart string
			switch {
			case f.editingKey:
				f.keyInput.Width = hKeyW
				keyPart = useFieldEditingStyle.Width(hKeyW).Render(f.keyInput.View())
				valPart = useFieldStyle.Width(hValW).Render(valDisplay)
			case f.editing:
				f.input.Width = hValW
				keyPart = useFieldActiveStyle.Width(hKeyW).Render(keyDisplay)
				valPart = useFieldEditingStyle.Width(hValW).Render(f.input.View())
			case isActive:
				keyPart = useFieldActiveStyle.Width(hKeyW).Render(keyDisplay)
				valPart = useFieldActiveStyle.Width(hValW).Render(valDisplay)
			default:
				keyPart = useFieldStyle.Width(hKeyW).Render(keyDisplay)
				valPart = useFieldStyle.Width(hValW).Render(valDisplay)
			}

			sep := " "
			// Fixed-width label placeholder so column never shifts
			lbl := useLabelStyle.Render("")
			lines = append(lines, lipgloss.JoinHorizontal(lipgloss.Center, lbl, keyPart, sep, valPart))
		} else {
			// params / query / body field
			lbl := useLabelStyle.Render(f.key) // always fixed width via useLabelStyle.Width(18)
			displayVal := f.value

			// Var resolution indicator — rendered in its own column after the field
			var varIndicator string
			if !f.editing && strings.Contains(f.value, "{{") {
				if s := strings.Index(f.value, "{{"); s >= 0 {
					if e := strings.Index(f.value, "}}"); e > s {
						name := f.value[s+2 : e]
						found := false
						for _, v := range m.vars {
							if v.Name == name {
								found = true
								break
							}
						}
						if found {
							varIndicator = "  " + lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Render("✓")
						} else {
							varIndicator = "  " + lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render("✗") +
								" " + helpLook.Render("v")
						}
					}
				}
			}

			var fieldPart string
			switch {
			case f.editing:
				f.input.Width = rowW
				fieldPart = useFieldEditingStyle.Width(rowW).Render(f.input.View())
			case isActive:
				if displayVal == "" {
					displayVal = usePlaceholderStyle.Render("enter value")
				}
				fieldPart = useFieldActiveStyle.Width(rowW).Render(displayVal)
			default:
				if displayVal == "" {
					displayVal = usePlaceholderStyle.Render("enter value")
				}
				fieldPart = useFieldStyle.Width(rowW).Render(displayVal)
			}

			// varIndicator is appended after the fixed-width field, never inside or attached to label
			lines = append(lines, lipgloss.JoinHorizontal(lipgloss.Top, lbl, fieldPart, varIndicator))
		}

		// Render suggestion dropdown immediately below the active row
		if isActive && f.showSuggestion && len(activeSuggestions) > 0 {
			maxSugg := 6
			if len(activeSuggestions) < maxSugg {
				maxSugg = len(activeSuggestions)
			}
			suggOffset := labelW + 2
			if f.section == "headers" && f.editing {
				// Align under value column
				suggOffset += hKeyW + 5
			}
			indent := strings.Repeat(" ", suggOffset)
			for si := 0; si < maxSugg; si++ {
				item := activeSuggestions[si]
				var rendered string
				if si == activeSuggestionIdx {
					rendered = suggestionActiveStyle.Render(item)
				} else {
					rendered = suggestionNormalStyle.Render(item)
				}
				lines = append(lines, indent+rendered)
			}
		}
	}

	// ── URL / curl preview ──────────────────────────────────────────────────
	lines = append(lines, "")
	resolvedFields := resolveFields(m.useFields, m.vars)
	builtURL := buildURL(m.cfg.BaseURL, ep.Path, resolvedFields)
	urlLine := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("url  ") +
		curlPreviewStyle.Render(builtURL)
	lines = append(lines, urlLine)

	// ── Send hint ──────────────────────────────────────────────────────────
	lines = append(lines, "")
	lines = append(lines, fireHintStyle.Render(" ctrl+r ")+" "+helpLook.Render("send")+"  "+fireHintStyle.Render(" y ")+" "+helpLook.Render("copy curl"))
	lines = append(lines, "")

	// ── Divider ────────────────────────────────────────────────────────────
	lines = append(lines, dividerStyle.Render(strings.Repeat("─", width-4)))
	lines = append(lines, "")

	// ── Response ───────────────────────────────────────────────────────────
	if m.useFiring {
		lines = append(lines, firingStyle.Render("firing…"))
	} else if m.useResponse != nil {
		resp := m.useResponse
		if resp.err != nil {
			lines = append(lines, respErrStyle.Render("error  ")+helpLook.Render(resp.err.Error()))
		} else {
			statusStr := fmt.Sprintf("%d", resp.statusCode)
			statusStyle := respOKStyle
			if resp.statusCode >= 400 {
				statusStyle = respErrStyle
			}
			elapsed := fmt.Sprintf("%dms", resp.elapsed.Milliseconds())
			lines = append(lines,
				respHeaderStyle.Render("response  ")+
					statusStyle.Render(statusStr)+
					helpLook.Render("  "+elapsed),
			)
			lines = append(lines, "")
			bodyLines := strings.Split(resp.body, "\n")
			for _, bl := range bodyLines {
				lines = append(lines, respBodyStyle.Render(bl))
			}
		}
	} else {
		lines = append(lines, helpLook.Render("no response  ctrl+r to send"))
	}

	// ── Scroll & clip ──────────────────────────────────────────────────────
	allContent := strings.Join(lines, "\n")
	allLines := strings.Split(allContent, "\n")

	scroll := m.useScroll
	if scroll > len(allLines)-1 {
		scroll = len(allLines) - 1
	}
	if scroll < 0 {
		scroll = 0
	}
	allLines = allLines[scroll:]

	visibleHeight := height - 2
	if len(allLines) > visibleHeight {
		allLines = allLines[:visibleHeight]
	}

	return detailPad.Width(width).Render(strings.Join(allLines, "\n"))
}

// wrapString wraps a string at maxWidth characters.
func wrapString(s string, maxWidth int) []string {
	if maxWidth <= 0 {
		return []string{s}
	}
	var result []string
	for len(s) > maxWidth {
		result = append(result, s[:maxWidth])
		s = "  " + s[maxWidth:] // indent continuation
	}
	result = append(result, s)
	return result
}

// ---------------------------------------------------------------------------
// Sidebar / search / detail renderers (unchanged from original)
// ---------------------------------------------------------------------------

func (m model) renderSidebar(width, height int) string {
	var sb strings.Builder

	sb.WriteString(titleLook.Render(m.cfg.Title))
	if m.version != "" {
		sb.WriteString("  ")
		sb.WriteString(helpLook.Render("rute " + m.version))
	}
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
		method := renderer.MethodStyle(ep.Method)
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
