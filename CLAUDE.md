
# rute — AI Context File

> Read this at the start of every session before doing anything.
> Keep this file updated as decisions are made and things change.

---

## What this project is

**rute** is a lightweight, Zod-native API documentation tool for TypeScript projects.

The name comes from the Danish word for "route" — built by a Danish developer, named to reflect the core concept of API routes.

**GitHub description:** Browse and export your API routes and Zod schemas from the terminal.

The core idea: developers already write Zod schemas for runtime validation. Those schemas are the source of truth for what an API accepts and returns. Instead of maintaining separate documentation that drifts from reality, rute reads those schemas directly and renders them as human-readable documentation — in the terminal, and eventually as an exportable static HTML site.

**The workflow:**
1. Developer writes Zod schemas as normal (for validation)
2. A lightweight `rute.yaml` file in the repo root links routes to those schemas
3. Anyone who clones the repo can run `rute` and browse the API docs instantly in their terminal
4. `rute export` generates a clean static HTML site for public-facing documentation

**The tagline:** Like a README but for your API. Docs that live with the code, never drift from it.

---

## Why we're building this

Existing tools (zod-to-openapi, Swagger, Redoc) all produce heavy OpenAPI/JSON output. They solve publishing, not co-location. Nobody has built a CLI-first, schema-linked, zero-overhead documentation tool that treats the terminal as a first-class citizen and the repo as the home for docs.

The gap:
- Zod schemas already describe your API shape perfectly
- Swagger/OpenAPI is too heavy for most TypeScript projects
- No tool currently lets you browse API docs from inside the repo without a browser
- No tool unfolds Zod validation rules (min, max, email, uuid, optional) into readable output

---

## Target user

A TypeScript developer who:
- Is already using Zod for validation
- Works in a team on an API
- Lives in the terminal
- Is annoyed that docs and code drift apart
- Doesn't want to set up a full Swagger infrastructure for a mid-sized API

---

## Tech stack

| Layer | Choice | Reason |
|-------|--------|--------|
| CLI/TUI | Go | Single binary, fast startup, great TUI libraries, easy distribution |
| TUI library | Bubble Tea (charmbracelet) | Best-in-class Go TUI framework |
| Styling | Lip Gloss (charmbracelet) | Pairs with Bubble Tea, clean API |
| YAML parsing | gopkg.in/yaml.v3 | Standard Go YAML library |
| Zod parser | Node.js extractor script | Go can't natively run TS, small JS script extracts schema shape and outputs JSON for Go to consume |
| CLI framework | Cobra | Standard Go CLI framework |
| Build/release | GoReleaser | Cross-platform binaries, Homebrew tap |
| CI | GitHub Actions | Build + test on push |

---

## Repository structure

```
rute/
├── cmd/
│   └── rute/
│       └── main.go          # Entrypoint
├── internal/
│   ├── yaml/                # rute.yaml reader and validator
│   ├── parser/              # Zod schema parser (calls Node.js extractor)
│   ├── renderer/            # Terminal tree renderer
│   ├── tui/                 # Bubble Tea TUI components
│   └── export/              # HTML export generator
├── extractor/
│   └── index.js             # Node.js script that parses Zod schemas and outputs JSON
├── testdata/
│   └── schemas/             # Fixture .ts files for parser tests
├── docs/
│   └── SPEC.md              # rute.yaml format specification
├── .github/
│   └── workflows/
│       └── ci.yml           # GitHub Actions CI
├── CLAUDE.md                # This file
├── README.md
├── LICENSE                  # MIT
├── Makefile
└── go.mod
```

---

## The rute.yaml format

This is the core contract of the project. See `docs/SPEC.md` for the full specification.

Quick example:

```yaml
title: My API
version: 1.0.0
baseUrl: https://api.example.com

endpoints:
  - path: /users/:id
    method: GET
    description: Get a user by ID
    params:
      schema: ./schemas/user.ts#UserParamsSchema
    response:
      200:
        schema: ./schemas/user.ts#UserResponseSchema
      404:
        schema: ./schemas/errors.ts#NotFoundSchema

  - path: /users
    method: POST
    description: Create a new user
    body:
      schema: ./schemas/user.ts#CreateUserSchema
    response:
      201:
        schema: ./schemas/user.ts#UserResponseSchema
```

Schema references follow the format: `<relative path to .ts file>#<exported schema name>`

---

## CLI commands

| Command | Description | Milestone |
|---------|-------------|-----------|
| `rute` | Launch interactive TUI browser | 2 |
| `rute list` | List all endpoints (method, path, description) | 1 |
| `rute show <path>` | Show full detail for one endpoint with unfolded schema trees | 1 |
| `rute validate` | Check all schema references resolve | 2 |
| `rute init` | Scaffold a starter rute.yaml | 2 |
| `rute export` | Generate static HTML documentation site | 3 |
| `rute export --watch` | Watch for changes and live reload | 3 |
| `rute publish` | Upload exported docs to public hosting | 4 |

---

## How the Zod parser works

This is the hardest part of the project. Go cannot natively import TypeScript files, so we use a small Node.js extractor script.

**Flow:**
1. Go reads `rute.yaml` and finds a schema reference e.g. `./schemas/user.ts#UserSchema`
2. Go shells out to `node extractor/index.js ./schemas/user.ts UserSchema`
3. The Node.js script parses the `.ts` file, finds the named export, traverses the Zod schema tree, and outputs a JSON representation of the schema shape
4. Go reads the JSON, builds an internal schema struct, and renders it

**Extractor JSON output format:**
```json
{
  "name": "UserSchema",
  "type": "object",
  "fields": [
    { "name": "id", "type": "string", "required": true, "validations": ["uuid"] },
    { "name": "email", "type": "string", "required": true, "validations": ["email"] },
    { "name": "age", "type": "number", "required": false, "validations": ["min:18", "max:120"] },
    { "name": "role", "type": "enum", "required": true, "values": ["admin", "user", "guest"] }
  ]
}
```

**Supported Zod types:**
- `z.string()` + validators: `.email()`, `.uuid()`, `.url()`, `.regex()`, `.min()`, `.max()`
- `z.number()` + validators: `.min()`, `.max()`, `.int()`, `.positive()`, `.negative()`
- `z.boolean()`
- `z.enum()`
- `z.object()` — recursive
- `z.array()` — with inner type
- `z.union()`
- `z.discriminatedUnion()`
- `z.intersection()`
- `z.record()`
- `z.optional()` / `.optional()`
- `z.nullable()`
- `.default()` — surface default value
- `.describe()` — use as field description

---

## Terminal rendering

Schema fields render as a tree:

```
GET /users/:id
Get a user by ID

Params
└── id  string  required  uuid

Response 200
├── id          string    required  uuid
├── email       string    required  email
├── age         number    optional  min:18 max:120
└── role        enum      required  "admin" | "user" | "guest"
```

HTTP method is colour coded:
- GET — green
- POST — blue
- PUT — yellow
- PATCH — yellow
- DELETE — red

---

## HTML export

Target quality: Stripe API docs. Clean, fast, readable, light + dark mode.

- Single self-contained `index.html` file (all CSS inlined, zero external dependencies)
- Sidebar with all endpoints, grouped optionally by tag
- Endpoint detail with schema rendered as clean tables
- Validation rules shown as small badges
- Light + dark mode
- Mobile responsive
- Optional code examples per endpoint (defined in `rute.yaml`)

Output: `./rute-out/index.html` by default. Configurable via `--out` flag.

---

## Milestones

### Milestone 1 — Core ✅ shipped

- [x] Issue 1: Define the YAML spec format → `docs/SPEC.md`
- [x] Issue 2: Bootstrap Go project structure
- [x] Issue 3: YAML file reader
- [x] Issue 4: Zod schema parser
- [x] Issue 5: Schema tree renderer
- [x] Issue 6: `rute list` command
- [x] Issue 7: `rute show <path>` command

### Milestone 2 — Polish ✅ shipped

- [x] Issue 8: Interactive TUI browser (Bubble Tea)
- [x] Issue 9: `rute validate` command
- [x] Issue 10: Support common Zod patterns
- [x] Issue 11: `rute init` command

### Milestone 3 — Export ✅ shipped

- [x] Issue 12: `rute export` static HTML generation
- [x] Issue 13: HTML doc site design
- [x] Issue 14: `rute export --watch` live reload

### Milestone 4 — Publish ✅ shipped

- [x] Issue 15: GoReleaser + Homebrew + install script
- [x] Issue 16: Public hosting + `rute publish` command

### Post-1.0 backlog (open issues)

- [ ] #19: Derive routes from the framework, not a hand-written `rute.yaml`
- [ ] #20: Machine-readable output (`--json`) so agents can explore the API
- [ ] #21: Test the Zod extractor + parser (highest-risk gap)
- [ ] #22: Support `z.tuple`, `z.enum(NativeEnum)`, `z.coerce.*`, `.readonly`
- [ ] #23: Document "use mode" (live request client) as a first-class feature

---

## Decisions made

| Decision | Choice | Reason | Date |
|----------|--------|--------|------|
| Language | Go | Single binary, fast startup, great TUI libs | SESSION 1 |
| TUI library | Bubble Tea | Best Go TUI framework | SESSION 1 |
| License | MIT | Maximise adoption, no friction | SESSION 1 |
| Zod parsing strategy | Node.js extractor script | Go can't run TS natively, small JS script is cleanest bridge | SESSION 1 |
| Output format | CLI-first, HTML export secondary | Core value is in-repo browsability | SESSION 1 |
| OpenAPI compatibility | None intentionally | Staying lightweight is the whole point | SESSION 1 |
| Project name | rute | Danish for "route", unique in CLI space, immediately clear to developers | SESSION 1 |

---

## Known gotchas and things to watch

- **Two extractor paths exist, and they can silently disagree.** `extractor/index.js` static-parses with zero dependencies; `extractor/runtime.js` bundles with esbuild, evaluates, and introspects via `z.toJSONSchema()`. Runtime falls back to static when esbuild/zod are missing or Zod < 4 (`runtime.js:57,62`), so the same `.ts` file can yield different output depending on the user's `node_modules`, with no signal. Runtime is the truth source (it sees the real schema object); a divergence is a static-extractor bug. This is what #21 exists to catch.
- **rute targets Zod 4** (`z.toJSONSchema()` is v4-only). Watch the v4 API changes: `z.nativeEnum()` deprecated in favour of `z.enum()` accepting native enum objects, and coercion moved to the `z.coerce.*` namespace.
- Composed schemas (`BaseSchema.extend({...})`, `.merge()`, `.pick()`, `.omit()`, `.partial()`) are handled at `extractor/index.js:118-155` with import following at :39-63. Cross-file `.merge()` and chained transforms have code paths but no tests yet.
- `z.lazy()` for recursive schemas is deliberately deferred.
- Go's `os/exec` to shell out to Node.js means Node must be installed on the user's machine. This is a reasonable assumption for a TypeScript developer but should be documented clearly.
- Test coverage is thin: only `internal/renderer` has tests. `internal/parser`, `internal/yaml`, `internal/export`, `internal/tui` and both extractors have none.

---

## Current focus

> Update this section at the start and end of every session.

**Status:** Milestones 1 through 4 all shipped. Released v0.1.1 → v0.1.5, Homebrew tap live. Every planned command works: `list`, `show`, `validate`, `init`, `export` (+ `--watch`), `publish`, and the TUI.

Beyond the original plan, the TUI gained a **live request client** ("use mode"): send real HTTP requests with editable headers and body, copy as curl. Undocumented and unadvertised, see #23.

**Completed this session (audit only, no code changes):**
- Audited all 11 open issues against the actual code, verified by build, `go test`, and running the binary
- Closed #10–#18 as shipped, each with evidence in the closing comment
- Opened #21 (extractor tests), #22 (missing Zod 4 types), #23 (document use mode)
- Amended #21 to specify a differential harness between the two extractor paths, and #22 to target the real Zod 4 API surface rather than deprecated `z.nativeEnum`
- Corrected this file: milestones, gotchas, current focus

**Backlog, in recommended order:**
1. **#21** extractor + parser tests. Highest risk: rute's value is faithfulness to the real schemas, and a parse bug is silently wrong rather than loud. Build the differential harness first.
2. **#22** missing Zod 4 types. Nearly free once #21's harness exists, one fixture per type.
3. **#19** derive routes from the framework. The differentiator: kills `rute.yaml` as a second source of truth.
4. **#23** document use mode. Cheap, fixes positioning.
5. **#20** `--json` output. Cheap, stacks on the existing structs.

---

## Session log

> Add a one-line note after each session so we have a rough history.

- **Session 1:** Full project planning. Defined concept, stack, milestones, issues, YAML format, parser strategy. Repo not yet created.
- **Session 2:** GitHub setup (labels, milestones, 16 issues). Implemented Milestone 1 end-to-end: SPEC.md, Go project structure, YAML reader, Zod extractor (Node.js), parser bridge, terminal renderer, `rute list`, `rute show`. All working.
- **Session 3:** Implemented Milestones 2–4 in one pass (commit `1841bbc`): TUI, validate, init, export + HTML template, publish, GoReleaser + Homebrew + install.sh. Added runtime extractor path (`extractor/runtime.js`). Released v0.1.1 → v0.1.5. Docs site + installation guide. Issues left open.
- **Session 4:** Audit + housekeeping. Confirmed M1–M4 shipped, closed #10–#18 with evidence, opened and refined #21–#23, updated this file (milestones, gotchas, current focus). Found two things the plan missed: the two extractor paths can silently disagree, and rute targets Zod 4 so `z.nativeEnum` is the wrong target while `z.enum(NativeEnum)` and `z.coerce.*` are the real gaps.
