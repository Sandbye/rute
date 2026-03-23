# rute installation

Use this file when you want a human or an agent to install `rute` without guessing.

If you are instructing an agent, point it at this file directly:

```text
Read docs/installation.md and install rute.
```

## Preferred install

Use Homebrew on macOS:

```sh
brew install Sandbye/rute/rute
```

## Script install

Use the install script when Homebrew is not the right fit:

```sh
curl -fsSL https://raw.githubusercontent.com/Sandbye/rute/main/install.sh | sh
```

## Build from source

Use this when developing `rute` itself or when you want a local build:

```sh
git clone https://github.com/Sandbye/rute
cd rute
make build
mv rute /usr/local/bin/
```

## Verify the install

Check that the binary is available:

```sh
rute --version
```

Then run it inside a project that contains a `rute.yaml` file:

```sh
rute
```

## Agent notes

If an agent is installing `rute`, it should:

1. Read this file first.
2. Prefer Homebrew on macOS.
3. Fall back to the install script if Homebrew is unavailable.
4. Use a source build only when explicitly requested or when working on the `rute` repo itself.
5. Run `rute --version` after installation to confirm the binary is on `PATH`.
