# rute

Browse and export your API routes and Zod schemas from the terminal.

rute reads a `rute.yaml` file in your repo and renders your API documentation directly in the terminal — no browser, no build step, no drift from the actual schemas.

## Install

### Homebrew
```sh
brew install Sandbye/rute/rute
```

### Install script
```sh
curl -fsSL https://raw.githubusercontent.com/Sandbye/rute/main/install.sh | sh
```

### Build from source
```sh
git clone https://github.com/Sandbye/rute
cd rute
make build
mv rute /usr/local/bin/
```

## Quick start

1. Add a `rute.yaml` to your project root:
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

2. Run rute:
```sh
# List all endpoints
rute list

# Inspect a specific endpoint
rute show /users/:id

# Launch the interactive TUI browser
rute

# Generate HTML documentation
rute export

# Watch for changes and rebuild HTML
rute export --watch

# Validate all schema references
rute validate
```

## Learn more

- [`docs/SPEC.md`](docs/SPEC.md) — Full `rute.yaml` format specification
- [`rute --help`](https://github.com/Sandbye/rute#readme) — All commands and flags

## License

MIT