package export

const htmlTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}} — API Documentation</title>
<style>
*, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }

:root {
  --bg: #ffffff;
  --bg-sidebar: #f8fafc;
  --bg-card: #ffffff;
  --bg-code: #f8fafc;
  --bg-badge: #f1f5f9;
  --bg-hover: #f1f5f9;
  --text: #0f172a;
  --text-secondary: #475569;
  --text-dim: #94a3b8;
  --border: #e2e8f0;
  --border-light: #f1f5f9;
  --accent: #3b82f6;
  --tree: #cbd5e1;
  --font: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
  --mono: 'SF Mono', 'Fira Code', 'Fira Mono', Menlo, Consolas, monospace;
  --sidebar-w: 300px;
  --radius: 6px;
}

@media (prefers-color-scheme: dark) {
  :root {
    --bg: #0b1120;
    --bg-sidebar: #111827;
    --bg-card: #1e293b;
    --bg-code: #1e293b;
    --bg-badge: #334155;
    --bg-hover: #1e293b;
    --text: #f1f5f9;
    --text-secondary: #cbd5e1;
    --text-dim: #64748b;
    --border: #1e293b;
    --border-light: #1e293b;
    --tree: #475569;
  }
}

html { scroll-behavior: smooth; }
body {
  font-family: var(--font);
  color: var(--text);
  background: var(--bg);
  line-height: 1.6;
  -webkit-font-smoothing: antialiased;
}

/* ── Sidebar ── */
.sidebar {
  position: fixed;
  top: 0; left: 0; bottom: 0;
  width: var(--sidebar-w);
  background: var(--bg-sidebar);
  border-right: 1px solid var(--border);
  overflow-y: auto;
  z-index: 10;
  display: flex;
  flex-direction: column;
}

.sidebar-header {
  padding: 24px 20px 16px;
  border-bottom: 1px solid var(--border);
  flex-shrink: 0;
}

.sidebar-header h1 {
  font-size: 16px;
  font-weight: 700;
  letter-spacing: -0.02em;
}

.sidebar-header .meta {
  display: flex;
  gap: 8px;
  align-items: center;
  margin-top: 4px;
}

.sidebar-header .version {
  font-size: 11px;
  font-family: var(--mono);
  color: var(--text-dim);
  background: var(--bg-badge);
  padding: 1px 6px;
  border-radius: 3px;
}

.search-trigger {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  margin-top: 12px;
  padding: 7px 10px;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  background: var(--bg);
  color: var(--text-dim);
  font-size: 13px;
  font-family: var(--font);
  cursor: pointer;
  transition: border-color 0.15s;
}

.search-trigger:hover { border-color: var(--text-dim); }

.search-trigger span { flex: 1; text-align: left; }

.search-trigger kbd {
  font-family: var(--mono);
  font-size: 10px;
  padding: 1px 5px;
  border-radius: 3px;
  background: var(--bg-badge);
  border: 1px solid var(--border);
  color: var(--text-dim);
}

.sidebar-nav { padding: 12px 0; overflow-y: auto; flex: 1; }

.group-label {
  font-size: 11px;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.1em;
  color: var(--text-dim);
  padding: 12px 20px 6px;
}

.sidebar-nav a {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 5px 20px;
  text-decoration: none;
  color: var(--text-secondary);
  font-size: 13px;
  transition: background 0.1s, color 0.1s;
}

.sidebar-nav a:hover {
  background: var(--bg-hover);
  color: var(--text);
}

.method-pill {
  display: inline-block;
  font-family: var(--mono);
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 0.04em;
  padding: 2px 0;
  min-width: 38px;
  text-align: right;
  flex-shrink: 0;
}

.sidebar-nav .path {
  font-family: var(--mono);
  font-size: 12px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

/* ── Main ── */
.main {
  margin-left: var(--sidebar-w);
  display: flex;
  justify-content: center;
  min-height: 100vh;
}

.content {
  width: 100%;
  max-width: 720px;
  padding: 48px 32px 120px;
}

.page-header {
  margin-bottom: 48px;
  padding-bottom: 24px;
  border-bottom: 1px solid var(--border);
}

.page-header h1 {
  font-size: 26px;
  font-weight: 700;
  letter-spacing: -0.02em;
}

.page-header p {
  color: var(--text-secondary);
  margin-top: 6px;
  font-size: 15px;
}

.page-header .base-url {
  font-family: var(--mono);
  font-size: 12px;
  color: var(--text-dim);
  margin-top: 8px;
}

/* ── Group header ── */
.group-header {
  font-size: 12px;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.1em;
  color: var(--text-dim);
  padding: 32px 0 12px;
  border-bottom: 1px solid var(--border-light);
  margin-bottom: 24px;
}

.group-header:first-of-type { padding-top: 0; }

/* ── Endpoint ── */
.endpoint {
  margin-bottom: 48px;
  scroll-margin-top: 24px;
}

.ep-header {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 6px;
}

.ep-method {
  font-family: var(--mono);
  font-size: 13px;
  font-weight: 700;
  letter-spacing: 0.02em;
}

.ep-path {
  font-family: var(--mono);
  font-size: 17px;
  font-weight: 600;
  letter-spacing: -0.01em;
}

.ep-desc {
  color: var(--text-secondary);
  font-size: 14px;
  margin-bottom: 20px;
}

/* ── Sections ── */
.section { margin-bottom: 20px; }

.section-label {
  font-size: 11px;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.08em;
  color: var(--text-dim);
  margin-bottom: 8px;
}

/* ── Schema tree (matches TUI aesthetic) ── */
.schema-tree {
  font-family: var(--mono);
  font-size: 13px;
  line-height: 1.7;
  background: var(--bg-code);
  border-radius: var(--radius);
  padding: 14px 16px;
  overflow-x: auto;
}

.field-row {
  display: flex;
  align-items: baseline;
  gap: 0;
  white-space: nowrap;
}

.tree-guide {
  color: var(--tree);
  user-select: none;
  flex-shrink: 0;
}

.f-name { font-weight: 600; color: var(--text); }
.f-type { color: var(--accent); margin-left: 8px; }
.f-opt  { color: var(--text-dim); margin-left: 8px; }

.f-badge {
  display: inline-block;
  font-size: 11px;
  padding: 0 5px;
  margin-left: 6px;
  border-radius: 3px;
  background: var(--bg-badge);
  color: var(--text-secondary);
}

.f-desc {
  color: var(--text-dim);
  font-style: italic;
  margin-left: 8px;
  font-family: var(--font);
  font-size: 12px;
}

.f-enum-vals {
  color: var(--text-secondary);
  margin-left: 8px;
}

/* ── Examples ── */
.example-block {
  background: var(--bg-code);
  border-radius: var(--radius);
  padding: 14px 16px;
  margin-bottom: 10px;
  overflow-x: auto;
}

.example-label {
  font-size: 11px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  color: var(--text-dim);
  margin-bottom: 6px;
}

.example-block pre {
  font-family: var(--mono);
  font-size: 13px;
  line-height: 1.5;
  margin: 0;
}

/* ── Search overlay ── */
.search-backdrop {
  display: none;
  position: fixed;
  inset: 0;
  background: rgba(0,0,0,0.4);
  z-index: 100;
  justify-content: center;
  padding-top: 20vh;
}

.search-backdrop.open { display: flex; }

.search-box {
  width: 420px;
  max-width: 90vw;
  background: var(--bg-card);
  border: 1px solid var(--border);
  border-radius: 10px;
  box-shadow: 0 16px 40px rgba(0,0,0,0.2);
  overflow: hidden;
  align-self: flex-start;
}

.search-input-wrap {
  display: flex;
  align-items: center;
  padding: 12px 16px;
  border-bottom: 1px solid var(--border);
  gap: 10px;
}

.search-input-wrap svg {
  flex-shrink: 0;
  color: var(--text-dim);
}

.search-input {
  flex: 1;
  border: none;
  outline: none;
  background: transparent;
  font-family: var(--mono);
  font-size: 14px;
  color: var(--text);
}

.search-input::placeholder { color: var(--text-dim); }

.search-results {
  max-height: 300px;
  overflow-y: auto;
  padding: 4px 0;
}

.search-results:empty::after {
  content: 'No matches';
  display: block;
  padding: 16px;
  color: var(--text-dim);
  font-size: 13px;
  text-align: center;
}

.search-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 16px;
  cursor: pointer;
  text-decoration: none;
  color: var(--text-secondary);
  font-size: 13px;
}

.search-item:hover, .search-item.active {
  background: var(--bg-hover);
  color: var(--text);
}

.search-item .method-pill {
  font-family: var(--mono);
  font-size: 10px;
  font-weight: 700;
  min-width: 38px;
  text-align: right;
}

.search-item .path { font-family: var(--mono); font-size: 12px; }
.search-item .desc { font-size: 12px; color: var(--text-dim); margin-left: auto; }

.search-hint {
  padding: 8px 16px;
  border-top: 1px solid var(--border);
  font-size: 11px;
  color: var(--text-dim);
  display: flex;
  gap: 16px;
}

.search-hint kbd {
  font-family: var(--mono);
  font-size: 10px;
  padding: 1px 5px;
  border-radius: 3px;
  background: var(--bg-badge);
  border: 1px solid var(--border);
}

/* ── Mobile ── */
@media (max-width: 768px) {
  :root { --sidebar-w: 0px; }
  .sidebar { display: none; }
  .main { margin-left: 0; }
  .content { padding: 24px 16px 64px; }
}
</style>
</head>
<body>

<aside class="sidebar">
  <div class="sidebar-header">
    <h1>{{.Title}}</h1>
    <div class="meta">
      {{if .Version}}<span class="version">v{{.Version}}</span>{{end}}
    </div>
    <button class="search-trigger" id="search-trigger">
      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/></svg>
      <span>Search...</span>
      <kbd id="search-shortcut"></kbd>
    </button>
  </div>
  <div class="sidebar-nav">
    {{range .Groups}}
    <div class="group-label">{{.Name}}</div>
    {{range .Endpoints}}
    <a href="#{{.ID}}">
      <span class="method-pill" style="color:{{methodColor .Method}}">{{.Method}}</span>
      <span class="path">{{.Path}}</span>
    </a>
    {{end}}
    {{end}}
  </div>
</aside>

<div class="main">
  <div class="content">
    <div class="page-header">
      <h1>{{.Title}}</h1>
      {{if .Description}}<p>{{.Description}}</p>{{end}}
      {{if .BaseURL}}<div class="base-url">{{.BaseURL}}</div>{{end}}
    </div>

    {{range .Groups}}
    <div class="group-header">{{.Name}}</div>

    {{range .Endpoints}}
    <div class="endpoint" id="{{.ID}}">
      <div class="ep-header">
        <span class="ep-method" style="color:{{methodColor .Method}}">{{.Method}}</span>
        <span class="ep-path">{{.Path}}</span>
      </div>
      {{if .Description}}<div class="ep-desc">{{.Description}}</div>{{end}}

      {{if .Params}}
      <div class="section">
        <div class="section-label">Path Parameters</div>
        <div class="schema-tree">{{template "fieldTree" .Params}}</div>
      </div>
      {{end}}

      {{if .Query}}
      <div class="section">
        <div class="section-label">Query Parameters</div>
        <div class="schema-tree">{{template "fieldTree" .Query}}</div>
      </div>
      {{end}}

      {{if .Body}}
      <div class="section">
        <div class="section-label">Request Body</div>
        <div class="schema-tree">{{template "fieldTree" .Body}}</div>
      </div>
      {{end}}

      {{range .Responses}}
      <div class="section">
        <div class="section-label">Response {{.Code}}</div>
        {{if .Schema}}<div class="schema-tree">{{template "fieldTree" .Schema}}</div>{{end}}
      </div>
      {{end}}

      {{if .Examples}}
      <div class="section">
        <div class="section-label">Examples</div>
        {{range .Examples}}
        <div class="example-block">
          <div class="example-label">{{if .Label}}{{.Label}}{{else}}{{.Lang}}{{end}}</div>
          <pre>{{.Code}}</pre>
        </div>
        {{end}}
      </div>
      {{end}}
    </div>
    {{end}}
    {{end}}
  </div>
</div>

{{define "fieldTree"}}{{range $i, $f := .Fields}}{{template "fieldLine" (makeFieldCtx $f $i (len $.Fields) "")}}{{end}}{{end}}

{{define "fieldLine"}}
<div class="field-row">
  <span class="tree-guide">{{.Prefix}}{{if .IsLast}}└── {{else}}├── {{end}}</span>
  <span class="f-name">{{.Field.Name}}</span>
  <span class="f-type">{{fieldType .Field}}</span>
  {{if not .Field.Required}}<span class="f-opt">optional</span>{{end}}
  {{if .Field.Nullable}}<span class="f-opt">nullable</span>{{end}}
  {{range .Field.Validations}}<span class="f-badge">{{.}}</span>{{end}}
  {{if hasDefault .Field.Default}}<span class="f-badge">default: {{defaultValue .Field.Default}}</span>{{end}}
  {{if .Field.Description}}<span class="f-desc">{{.Field.Description}}</span>{{end}}
</div>
{{if .Field.Fields}}{{range $i, $child := .Field.Fields}}{{template "fieldLine" (makeFieldCtx $child $i (len $.Field.Fields) (childPrefix $.Prefix $.IsLast))}}{{end}}{{end}}
{{end}}

<div class="search-backdrop" id="search">
  <div class="search-box">
    <div class="search-input-wrap">
      <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/></svg>
      <input class="search-input" id="search-input" type="text" placeholder="Search endpoints..." autocomplete="off" spellcheck="false">
    </div>
    <div class="search-results" id="search-results"></div>
    <div class="search-hint">
      <span><kbd>↑</kbd><kbd>↓</kbd> navigate</span>
      <span><kbd>↵</kbd> jump</span>
      <span><kbd>esc</kbd> close</span>
    </div>
  </div>
</div>

<script>
(function(){
  var endpoints = [
    {{range .Groups}}{{range .Endpoints}}
    {id:"{{.ID}}",method:"{{.Method}}",path:"{{.Path}}",desc:"{{.Description}}",color:"{{methodColor .Method}}"},
    {{end}}{{end}}
  ];

  var isMac = /Mac|iPhone|iPad/.test(navigator.platform || navigator.userAgent);
  document.getElementById('search-shortcut').textContent = isMac ? '⌘K' : 'Ctrl K';
  document.getElementById('search-trigger').onclick = function(){ open(); };

  var backdrop = document.getElementById('search');
  var input = document.getElementById('search-input');
  var results = document.getElementById('search-results');
  var active = 0;

  function open() {
    backdrop.classList.add('open');
    input.value = '';
    active = 0;
    render(endpoints);
    setTimeout(function(){ input.focus(); }, 10);
  }

  function close() {
    backdrop.classList.remove('open');
    input.value = '';
  }

  function render(items) {
    results.innerHTML = '';
    items.forEach(function(ep, i) {
      var a = document.createElement('a');
      a.className = 'search-item' + (i === active ? ' active' : '');
      a.href = '#' + ep.id;
      a.innerHTML = '<span class="method-pill" style="color:'+ep.color+'">'+ep.method+'</span>' +
        '<span class="path">'+ep.path+'</span>' +
        (ep.desc ? '<span class="desc">'+ep.desc+'</span>' : '');
      a.onclick = function(){ close(); };
      results.appendChild(a);
    });
  }

  function filtered() {
    var q = input.value.toLowerCase();
    if (!q) return endpoints;
    return endpoints.filter(function(ep) {
      return (ep.method + ' ' + ep.path + ' ' + ep.desc).toLowerCase().indexOf(q) >= 0;
    });
  }

  input.addEventListener('input', function() {
    active = 0;
    render(filtered());
  });

  document.addEventListener('keydown', function(e) {
    if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
      e.preventDefault();
      if (backdrop.classList.contains('open')) close(); else open();
      return;
    }
    if (e.key === '/' && !backdrop.classList.contains('open') && document.activeElement.tagName !== 'INPUT') {
      e.preventDefault();
      open();
      return;
    }
    if (!backdrop.classList.contains('open')) return;

    var items = filtered();
    if (e.key === 'Escape') { close(); return; }
    if (e.key === 'ArrowDown') { e.preventDefault(); active = Math.min(active+1, items.length-1); render(items); return; }
    if (e.key === 'ArrowUp') { e.preventDefault(); active = Math.max(active-1, 0); render(items); return; }
    if (e.key === 'Enter' && active >= 0 && active < items.length) {
      e.preventDefault();
      window.location.hash = '#' + items[active].id;
      close();
    }
  });

  backdrop.addEventListener('click', function(e) {
    if (e.target === backdrop) close();
  });
})();
</script>

</body>
</html>`
