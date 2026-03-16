# rute

Browse your API routes and Zod schemas from the terminal.

rute reads a `rute.yaml` file in your repo and renders your API documentation directly in the terminal — no browser, no build step, no drift from the actual schemas.

---

## Requirements

- **Go 1.21+** — to build the binary
- **Node.js** — to run the Zod schema extractor (any recent LTS version works)

---

## Install

### Build from source

```sh
git clone https://github.com/Sandbye/rute
cd rute
make build
```

This produces a `rute` binary in the repo root. Move it somewhere on your `$PATH`:

```sh
mv rute /usr/local/bin/rute
```

---

## Quick start

**1. Add a `rute.yaml` to your project root:**

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

Schema references point to named Zod exports in your `.ts` files:
```
./relative/path/to/file.ts#ExportedSchemaName
```

**2. Run rute from your project root:**

```sh
rute list
```

```
GET   /users/:id   Get a user by ID
POST  /users       Create a new user
```

**3. Inspect a specific endpoint:**

```sh
rute show /users/:id
```

```
GET /users/:id
Get a user by ID

Params
└── id  string  uuid

Response 200
├── id        string  uuid
├── email     string  email
├── age       number  optional  min:18 max:120
└── role      enum    "admin" | "user" | "guest"

Response 404
├── error      string
├── message    string
└── statusCode number  int
```

---

## Commands

| Command | Description |
|---------|-------------|
| `rute list` | List all endpoints (method, path, description) |
| `rute show <path>` | Show full detail for one endpoint with schema trees |

**Options:**

```
--config <path>   Use a rute.yaml at a custom path (default: ./rute.yaml)
```

Example:
```sh
rute --config ./config/rute.yaml list
```

### Shell completion

`rute show` supports Tab completion — it reads your `rute.yaml` and suggests endpoint paths.

```sh
# Bash
source <(rute completion bash)

# Zsh
rute completion zsh > "${fpath[1]}/_rute" && exec zsh

# Fish
rute completion fish > ~/.config/fish/completions/rute.fish
```

Then type `rute show ` and hit Tab to see available endpoints.

---

## rute.yaml format

See [`docs/SPEC.md`](docs/SPEC.md) for the full specification.

**Supported Zod types:**

| Zod | Rendered as |
|-----|-------------|
| `z.string()` | `string` |
| `z.number()` | `number` |
| `z.boolean()` | `boolean` |
| `z.enum([...])` | `enum  "a" \| "b"` |
| `z.object({...})` | nested tree |
| `z.array(z.string())` | `array<string>` |
| `z.optional()` / `.optional()` | field marked `optional` |
| `.email()`, `.uuid()`, `.min()`, `.max()` etc. | shown as validation badges |
| `.describe("text")` | shown as inline description |
| `.default(value)` | default value surfaced |

---

## Testing the project

### Run the Go tests

```sh
make test
# or
go test ./...
```

### Test the extractor directly

The Node.js extractor parses Zod schemas statically and outputs JSON. You can run it standalone against any `.ts` file:

```sh
node extractor/index.js testdata/schemas/user.ts UserResponseSchema
```

Expected output:
```json
{
  "name": "UserResponseSchema",
  "type": "object",
  "fields": [
    { "name": "id",        "type": "string", "required": true,  "validations": ["uuid"] },
    { "name": "email",     "type": "string", "required": true,  "validations": ["email"] },
    { "name": "age",       "type": "number", "required": false, "validations": ["min:18", "max:120"] },
    { "name": "role",      "type": "enum",   "required": true,  "values": ["admin", "user", "guest"] },
    { "name": "createdAt", "type": "string", "required": true,  "description": "ISO 8601 timestamp" }
  ]
}
```

Try the other fixtures:

```sh
node extractor/index.js testdata/schemas/user.ts UserParamsSchema
node extractor/index.js testdata/schemas/user.ts CreateUserSchema
node extractor/index.js testdata/schemas/errors.ts NotFoundSchema
node extractor/index.js testdata/schemas/errors.ts ValidationErrorSchema
```

### End-to-end test with the test fixture

There is a ready-made `testdata/rute.yaml` that references the fixture schemas:

```sh
# Build first
make build

# List endpoints
./rute --config testdata/rute.yaml list

# Show endpoint detail
./rute --config testdata/rute.yaml show /users/:id
./rute --config testdata/rute.yaml show /users
```

---

## Project structure

```
rute/
├── cmd/rute/
│   ├── main.go        # Entrypoint, root Cobra command
│   ├── list.go        # rute list
│   └── show.go        # rute show
├── internal/
│   ├── yaml/          # rute.yaml reader and typed structs
│   ├── parser/        # Go↔Node bridge (calls extractor)
│   ├── renderer/      # Terminal tree renderer (Lip Gloss)
│   ├── tui/           # Bubble Tea TUI stub (Milestone 2)
│   └── export/        # HTML export stub (Milestone 3)
├── extractor/
│   └── index.js       # Node.js static Zod parser
├── testdata/
│   ├── rute.yaml      # Test fixture config
│   └── schemas/       # Fixture .ts files
├── docs/
│   └── SPEC.md        # rute.yaml format specification
├── Makefile
└── go.mod
```

---

## How it works

rute has no runtime dependency on TypeScript or Zod itself. Instead:

1. Go reads `rute.yaml` and finds schema references like `./schemas/user.ts#UserSchema`
2. Go shells out to `node extractor/index.js ./schemas/user.ts UserSchema`
3. The Node script parses the `.ts` file **statically** (no imports, no compilation) and outputs a JSON description of the schema shape
4. Go reads the JSON, builds internal structs, and renders the terminal output

This means parsing is fast and requires no `node_modules` in your project.

---

## License

MIT
