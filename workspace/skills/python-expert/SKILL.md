---
name: python-expert
description: "🐍 Write production Python -- FastAPI/Django APIs, async services, typed data pipelines, pytest suites, and proper packaging with pyproject.toml. Activate for any Python coding, debugging, architecture, or library selection."
---

# 🐍 Python Expert

Production-grade Python that is type-annotated, async-first, and properly packaged from the start. The Pythonic way first, then the general approach. Show the right tool for the job and explain why.

## Approach

1. **Start with type hints** -- annotate all function signatures. Use `from __future__ import annotations` for forward references. Modern syntax: `list[str]` not `List[str]`, `str | None` not `Optional[str]`. Type hints catch bugs before tests do and serve as documentation.
2. **Choose async-first for I/O** -- use `async def` for endpoints, database queries, and HTTP calls. FastAPI handles async natively. Use `asyncio.gather()` for concurrent I/O, not threads.
3. **Pick the right framework** -- FastAPI for APIs (auto-docs, validation, async), Django for full web apps (admin, ORM, auth), SQLAlchemy for database access (async sessions with `asyncpg`), Pydantic for data validation and settings.
4. **Test with pytest** -- use fixtures for setup, `@pytest.mark.parametrize` for table-driven tests, `conftest.py` for shared fixtures. Aim for behavior tests, not implementation tests.
5. **Package correctly** -- use `pyproject.toml` (not setup.py), manage dependencies with `uv` (fastest) or `poetry`. Pin production dependencies, use ranges for library dependencies.
6. **Handle errors explicitly** -- custom exception classes inheriting from domain-specific bases. Log with `structlog` or `logging` with structured format. Never catch bare `except:`.
7. **Optimize data work** -- use polars for large datasets (faster than pandas, lazy evaluation). Use numpy for numerical computation. Profile with `cProfile` or `py-spy` before optimizing.

## Examples

**FastAPI with Pydantic validation:**
```python
from pydantic import BaseModel, Field
from fastapi import FastAPI, HTTPException

class CreateUser(BaseModel):
    name: str = Field(min_length=1, max_length=100)
    email: str = Field(pattern=r"^[\w.-]+@[\w.-]+\.\w+$")

app = FastAPI()

@app.post("/users", status_code=201)
async def create_user(data: CreateUser) -> User:
    user = await db.create_user(data.name, data.email)
    if not user:
        raise HTTPException(404, "Failed to create user")
    return user
```

**Pytest with parametrize:**
```python
@pytest.mark.parametrize("input,expected", [
    ("hello", "HELLO"),
    ("", ""),
    ("cafe", "CAFE"),
])
def test_uppercase(input: str, expected: str) -> None:
    assert uppercase(input) == expected
```

## Common Patterns

- **`pydantic-settings`** for config: `class Settings(BaseSettings): db_url: str` reads from env vars automatically
- **Context managers** for resource cleanup: `async with db.session() as session:`
- **`functools.lru_cache`** for expensive pure functions, `@cached_property` for computed attributes
- **`pathlib.Path`** over `os.path` -- cleaner API, operator overloading with `/`
- **Dataclasses** for simple data containers without validation needs

**Alembic migration example:**
```python
# alembic/versions/001_add_users.py
def upgrade():
    op.create_table("users",
        sa.Column("id", sa.Integer, primary_key=True),
        sa.Column("email", sa.String(255), unique=True, nullable=False),
        sa.Column("created_at", sa.DateTime, server_default=sa.func.now()),
    )
    op.create_index("ix_users_email", "users", ["email"])

def downgrade():
    op.drop_table("users")
```

**Structured logging with structlog:**
```python
import structlog
structlog.configure(processors=[structlog.processors.JSONRenderer()])
log = structlog.get_logger()
log.info("user_created", user_id=42, email="a@b.com")
# Output: {"event": "user_created", "user_id": 42, "email": "a@b.com"}
```

**Dependency injection pattern:**
```python
from fastapi import Depends

async def get_db() -> AsyncGenerator[AsyncSession, None]:
    async with async_session() as session:
        yield session

async def get_user_service(db: AsyncSession = Depends(get_db)) -> UserService:
    return UserService(db)

@app.get("/users/{id}")
async def get_user(id: int, svc: UserService = Depends(get_user_service)) -> User:
    return await svc.get(id)
```

## Anti-Patterns

- Using `dict` where a dataclass or Pydantic model fits -- named fields catch typos and enable IDE autocomplete.
- Mutable default arguments (`def f(items=[])`) -- use `None` with a default inside the function body.
- Bare `except` or `except Exception` without re-raising -- this silences bugs and makes debugging impossible.
- `import *` -- pollutes namespace, breaks IDE analysis, and hides dependencies.

## Guidelines

- Follow PEP 8 with a formatter (`ruff format` or `black`). Consistent style eliminates review debates.
- Write docstrings in Google style for public functions. Skip obvious ones -- `def get_name(self) -> str` does not need a docstring.
- Use `ruff` for linting (replaces flake8, isort, pyflakes in one fast tool).

### Boundaries

- When suggesting libraries, consider the project's existing stack -- do not recommend FastAPI to a Django project without a clear reason.
- Data science recommendations should note data scale -- pandas works at 1M rows, polars or Spark beyond that.
- Security-sensitive code (auth, crypto) should use established libraries, not custom implementations.

## FastAPI Project Structure

```
myapp/
  pyproject.toml           -- Dependencies, tool config (ruff, pytest)
  src/myapp/
    __init__.py
    main.py                -- FastAPI app factory, lifespan, middleware
    config.py              -- pydantic-settings: Settings(BaseSettings)
    models/                -- SQLAlchemy ORM models
    schemas/               -- Pydantic request/response models
    api/
      routes/              -- Route modules (users.py, orders.py)
      deps.py              -- Dependency injection (get_db, get_current_user)
    services/              -- Business logic (no HTTP awareness)
    repositories/          -- Database queries (async SQLAlchemy)
  migrations/              -- Alembic versions
  tests/
    conftest.py            -- Shared fixtures (test DB, client)
    test_users.py          -- Behavior tests per feature
```

