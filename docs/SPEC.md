# rute.yaml — Format Specification

This document is the authoritative reference for the `rute.yaml` file format.

---

## Overview

`rute.yaml` lives in the root of a project repository. It declares the API endpoints and links each one to the Zod schemas that define its inputs and outputs. rute reads this file to render documentation without any additional build step.

---

## Top-level fields

```yaml
title: string          # required — human-readable name for the API
version: string        # required — semver or arbitrary string e.g. "1.0.0", "v2"
baseUrl: string        # optional — base URL e.g. "https://api.example.com"
description: string    # optional — short description shown in exported docs
endpoints:             # required — list of endpoint definitions (see below)
  - ...
```

### Example

```yaml
title: My API
version: 1.0.0
baseUrl: https://api.example.com
description: Internal REST API for the platform.

endpoints:
  - ...
```

---

## Endpoint definition

Each item under `endpoints` describes one API route.

```yaml
- path: string         # required — route path e.g. "/users/:id"
  method: string       # required — HTTP method: GET, POST, PUT, PATCH, DELETE
  description: string  # optional — one-line summary of what the endpoint does
  tags: [string]       # optional — group endpoints in exported docs
  params:              # optional — URL path and query parameters
    schema: SchemaRef
  query:               # optional — query string parameters
    schema: SchemaRef
  body:                # optional — request body (POST, PUT, PATCH)
    schema: SchemaRef
  response:            # optional — map of HTTP status codes to schemas
    200:
      schema: SchemaRef
    404:
      schema: SchemaRef
  examples:            # optional — code examples shown in exported docs
    - lang: string     # e.g. "curl", "typescript", "python"
      label: string    # optional display label
      code: string     # raw code string (multi-line YAML block scalar recommended)
```

### Rules

- `path` must start with `/`
- `method` is case-insensitive; stored and rendered in uppercase
- `params`, `query`, `body` each accept exactly one `schema` reference
- `response` keys are HTTP status codes as integers or strings (`200`, `"200"`)
- `tags` values are arbitrary strings; used for grouping in exported docs only

---

## Schema reference format

A schema reference (`SchemaRef`) points to a named Zod schema export in a TypeScript file.

**Format:**

```
./relative/path/to/file.ts#ExportedSchemaName
```

**Rules:**

- Path is relative to the location of `rute.yaml`
- Path must end in `.ts`
- The fragment (`#ExportedSchemaName`) is the exact name of the exported `const` or `export` in that file
- The referenced export must be a Zod schema (i.e. assigned from `z.*`)

**Examples:**

```yaml
schema: ./schemas/user.ts#UserSchema
schema: ./src/api/schemas.ts#CreateOrderSchema
schema: ./schemas/errors.ts#NotFoundError
```

---

## Supported Zod types

The following Zod types are recognised and rendered by rute:

| Zod expression | Rendered type |
|---|---|
| `z.string()` | `string` |
| `z.number()` | `number` |
| `z.boolean()` | `boolean` |
| `z.enum(["a", "b"])` | `enum` — values listed |
| `z.object({...})` | `object` — fields rendered as tree |
| `z.array(z.string())` | `array<string>` |
| `z.union([...])` | `union` — variants listed |
| `z.discriminatedUnion(key, [...])` | `union` — discriminated |
| `z.intersection(A, B)` | `intersection` |
| `z.record(z.string())` | `record<string>` |
| `z.optional(...)` / `.optional()` | field marked optional |
| `z.nullable(...)` / `.nullable()` | field marked nullable |
| `.default(value)` | default value surfaced |
| `.describe("text")` | used as field description |

### String validators rendered

`.email()`, `.uuid()`, `.url()`, `.cuid()`, `.regex()`, `.min(n)`, `.max(n)`, `.length(n)`, `.startsWith()`, `.endsWith()`

### Number validators rendered

`.min(n)`, `.max(n)`, `.int()`, `.positive()`, `.negative()`, `.nonnegative()`, `.nonpositive()`, `.multipleOf(n)`

---

## Full example

```yaml
title: User API
version: 2.1.0
baseUrl: https://api.example.com
description: Manages user accounts and authentication.

endpoints:
  - path: /users/:id
    method: GET
    description: Get a user by ID
    tags: [users]
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
    tags: [users]
    body:
      schema: ./schemas/user.ts#CreateUserSchema
    response:
      201:
        schema: ./schemas/user.ts#UserResponseSchema
      422:
        schema: ./schemas/errors.ts#ValidationErrorSchema

  - path: /auth/login
    method: POST
    description: Authenticate and receive a session token
    tags: [auth]
    body:
      schema: ./schemas/auth.ts#LoginSchema
    response:
      200:
        schema: ./schemas/auth.ts#SessionSchema
      401:
        schema: ./schemas/errors.ts#UnauthorizedSchema
    examples:
      - lang: curl
        label: cURL
        code: |
          curl -X POST https://api.example.com/auth/login \
            -H "Content-Type: application/json" \
            -d '{"email": "user@example.com", "password": "secret"}'
      - lang: typescript
        label: TypeScript
        code: |
          const res = await fetch('/auth/login', {
            method: 'POST',
            body: JSON.stringify({ email, password }),
          })
```

---

## Validation

Running `rute validate` checks:

1. `rute.yaml` exists and is valid YAML
2. Required top-level fields (`title`, `version`, `endpoints`) are present
3. Each endpoint has `path` and `method`
4. Each `SchemaRef` resolves: the file exists, and the named export exists in it
5. Each referenced schema can be parsed by the extractor without errors

---

## File location

rute looks for `rute.yaml` in the current working directory. A custom path can be specified with the `--config` flag:

```
rute --config ./config/rute.yaml list
```
