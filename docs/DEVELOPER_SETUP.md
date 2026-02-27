# Sofia Developer Setup

This guide is for contributors who want to build, test, and iterate on Sofia locally.

## Prerequisites

- Go `1.25.7` or newer (see `go.mod`).
- `make`
- `git`
- Optional but recommended: `golangci-lint`

## Clone and bootstrap

```bash
git clone https://github.com/grasberg/sofia.git
cd sofia
make deps
```

## Build locally

```bash
make build
```

Artifacts:

- `build/sofia-<platform>-<arch>` (real binary)
- `build/sofia` (symlink to platform binary)

Run local binary:

```bash
./build/sofia version
./build/sofia onboard
./build/sofia agent -m "hello"
```

## Development workflow

### Run tests

```bash
make test
```

### Static analysis

```bash
make vet
```

### Lint and format

```bash
make fmt
make lint
```

Auto-fix lint issues where possible:

```bash
make fix
```

### Full local check

```bash
make check
```

## Build targets

- `make build` - build current platform.
- `make build-all` - build multi-platform binaries.
- `make install` - install to `~/.local/bin/sofia` by default.
- `make uninstall` - remove installed binary.
- `make uninstall-all` - remove binary and Sofia home data.
- `make clean` - remove `build/`.

## Project layout

- `cmd/sofia` - CLI entrypoint and command wiring (Cobra).
- `pkg/agent` - core agent loop, context, memory behavior.
- `pkg/providers` - model/provider adapters and routing.
- `pkg/channels` - Telegram/Discord integrations.
- `pkg/cron` - scheduled task service.
- `pkg/skills` - skill loading, discovery, installation.
- `docs/` - user-facing and design documentation.
- `workspace/` - default workspace templates/assets used by onboarding.

## Config and runtime notes for developers

- User config path: `~/.sofia/config.json`
- Default workspace path: `~/.sofia/workspace`
- `sofia onboard` generates local config/workspace for manual testing.

When testing provider changes, prefer `model_list` configuration over legacy `providers` fields.

## Common debug commands

```bash
./build/sofia agent --debug
./build/sofia gateway --debug
./build/sofia status
./build/sofia cron list
```

## Related docs

- End-user guide: `docs/USER_GUIDE.md`
- Tools configuration: `docs/tools_configuration.md`
- Model list migration: `docs/migration/model-list-migration.md`
- Provider design notes: `docs/design/provider-refactoring.md`
