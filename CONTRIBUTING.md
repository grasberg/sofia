# Contributing to Sofia

First off, thank you for considering contributing to Sofia! It's people like you that make Sofia such a great tool.

## 🚀 Quick Start

1. **Fork** the repository
2. **Clone** your fork: `git clone https://github.com/YOUR_USERNAME/sofia.git`
3. **Create a branch**: `git checkout -b my-feature`
4. **Make changes** and commit: `git commit -m "Add my feature"`
5. **Push**: `git push origin my-feature`
6. **Open a Pull Request**

## 🛠️ Development Setup

```bash
git clone https://github.com/grasberg/sofia.git
cd sofia
make deps && make build
make test
```

**Prerequisites:** Go 1.26+

## 📝 Commit Conventions

We follow [Conventional Commits](https://www.conventionalcommits.org/):

- `feat:` New feature
- `fix:` Bug fix
- `docs:` Documentation changes
- `style:` Code style changes (formatting, etc.)
- `refactor:` Code refactoring
- `test:` Adding or updating tests
- `chore:` Build process or auxiliary tool changes

## 🧪 Testing

Run the full test suite:

```bash
make test
```

Run tests for a specific package:

```bash
go test ./pkg/yourpackage/...
```

## 📋 Pull Request Process

1. Ensure all tests pass: `make test`
2. Update documentation if needed
3. Add tests for new features
4. Keep PRs focused — one feature per PR
5. Write a clear description of your changes

## 🏗️ Project Structure

```
sofia/
├── cmd/           # Command-line interface
├── pkg/           # Core packages
│   ├── agent/     # Agent orchestration
│   ├── llm/       # LLM provider abstraction
│   ├── memory/    # Memory and knowledge graph
│   ├── skills/    # Skill system
│   ├── tools/     # Built-in tools
│   └── gateway/   # Web UI and API
├── workspace/     # Default workspace files
├── Makefile       # Build and test commands
└── go.mod         # Go module definition
```

## 💡 Ways to Contribute

- **Bug reports** — Found a bug? Open an issue!
- **Feature requests** — Have an idea? We'd love to hear it.
- **Documentation** — Help improve our docs
- **Code** — Fix bugs, add features, improve performance
- **Skills** — Create and share new skills via ClawHub
- **Reviews** — Review open PRs

## 🤝 Code of Conduct

This project adheres to the [Contributor Covenant](CODE_OF_CONDUCT.md). By participating, you are expected to uphold this code.

## ❓ Questions?

Open a [Discussion](https://github.com/grasberg/sofia/discussions) — we're happy to help!

Thank you for contributing! 🎉