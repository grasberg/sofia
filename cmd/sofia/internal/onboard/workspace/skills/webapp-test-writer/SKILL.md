---
name: webapp-test-writer
description: Write Playwright TypeScript E2E tests for a web application from its source code. Reads the codebase, discovers routes/features/auth flows, and generates complete test files in tests/e2e/ inside the project.
---

# Webapp Test Writer

Use this skill when the user wants to generate Playwright E2E tests for a web application
they have locally. The user provides a path to the project directory; you read the code
and produce ready-to-run test files.

## Step 1 — Understand the project

Read the project to discover its structure. Run these in the project root:

```bash
# Framework / stack detection
cat package.json 2>/dev/null || cat requirements.txt 2>/dev/null || cat go.mod 2>/dev/null

# Existing test setup
ls tests/ e2e/ playwright.config.* 2>/dev/null

# Route/page discovery (adapt to framework)
# Next.js / Remix / SvelteKit
find . -type f -name "*.tsx" -o -name "*.jsx" | grep -E "(pages|app|routes)/" | head -40
# Express / Fastify / Hono
grep -r "router\.\|app\.get\|app\.post\|\.route(" --include="*.ts" --include="*.js" -l | head -20
# Generic: look for navigation links
grep -r "href=" --include="*.tsx" --include="*.jsx" --include="*.html" -h | grep -oP '(?<=href=")[^"#?]+' | sort -u | head -40
```

Then read the key files:
- Auth/login component or route (search for "login", "signin", "auth", "session", "cookie", "jwt")
- Main navigation / layout to map all major sections
- Any existing `.env.example` or README for base URL and test credentials

## Step 2 — Map what to test

Build a test plan before writing any code. List:

| Area | Route / Feature | Auth required? | Priority |
|------|----------------|---------------|----------|
| Auth | /login, /logout, /register | No | P1 |
| Core feature 1 | /dashboard | Yes | P1 |
| ... | ... | ... | ... |

Focus on **P1: critical user flows** — login, main feature, key CRUD actions.
Skip cosmetic pages (about, privacy) unless the user asks for them.

## Step 3 — Check/create Playwright config

Check if Playwright is already installed:

```bash
ls playwright.config.ts playwright.config.js 2>/dev/null
cat package.json | grep playwright
```

If not installed, create the minimal setup:

**`playwright.config.ts`** (project root):
```typescript
import { defineConfig, devices } from '@playwright/test';

export default defineConfig({
  testDir: './tests/e2e',
  timeout: 30_000,
  retries: 1,
  use: {
    baseURL: process.env.BASE_URL || 'http://localhost:3000',
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
  },
  projects: [
    { name: 'chromium', use: { ...devices['Desktop Chrome'] } },
  ],
});
```

Add to `package.json` scripts if not present:
```json
"test:e2e": "playwright test",
"test:e2e:ui": "playwright test --ui"
```

## Step 4 — Write the tests

Create `tests/e2e/` in the project. Structure:

```
tests/e2e/
├── fixtures/
│   └── auth.ts          # Reusable logged-in page fixture
├── auth.spec.ts         # Login, logout, register, auth guards
├── <feature>.spec.ts    # One file per major feature
└── navigation.spec.ts   # Nav links, 404 page
```

### Auth fixture pattern

Always create a reusable auth fixture so tests don't repeat login logic:

```typescript
// tests/e2e/fixtures/auth.ts
import { test as base, expect } from '@playwright/test';

export const test = base.extend<{ loggedInPage: Page }>({
  loggedInPage: async ({ page }, use) => {
    await page.goto('/login');
    await page.getByLabel(/email|username/i).fill(process.env.TEST_EMAIL || 'test@example.com');
    await page.getByLabel(/password/i).fill(process.env.TEST_PASSWORD || 'password');
    await page.getByRole('button', { name: /sign in|log in|submit/i }).click();
    await page.waitForURL(url => !url.pathname.includes('login'));
    await use(page);
  },
});
export { expect };
```

### Test file pattern

```typescript
// tests/e2e/auth.spec.ts
import { test, expect } from '@playwright/test';

test.describe('Authentication', () => {
  test('user can log in with valid credentials', async ({ page }) => {
    await page.goto('/login');
    await page.getByLabel(/email/i).fill(process.env.TEST_EMAIL || 'test@example.com');
    await page.getByLabel(/password/i).fill(process.env.TEST_PASSWORD || 'password');
    await page.getByRole('button', { name: /log in|sign in/i }).click();
    await expect(page).not.toHaveURL(/login/);
    await expect(page.getByRole('navigation')).toBeVisible();
  });

  test('shows error on invalid credentials', async ({ page }) => {
    await page.goto('/login');
    await page.getByLabel(/email/i).fill('wrong@example.com');
    await page.getByLabel(/password/i).fill('wrongpassword');
    await page.getByRole('button', { name: /log in|sign in/i }).click();
    await expect(page.getByRole('alert')).toBeVisible();
  });

  test('redirects unauthenticated users to login', async ({ page }) => {
    await page.goto('/dashboard');
    await expect(page).toHaveURL(/login/);
  });
});
```

### Selector priority

Use selectors in this order (most to least preferred):

1. `getByRole('button', { name: 'Submit' })` — semantic, robust
2. `getByLabel('Email')` — for form inputs
3. `getByText('Save changes')` — for links and visible text
4. `getByTestId('submit-btn')` — if data-testid attributes exist
5. `locator('.class-name')` — last resort, fragile

### What to assert

| Action | Assert |
|--------|--------|
| Navigation | `expect(page).toHaveURL(...)` |
| Element visible | `expect(locator).toBeVisible()` |
| Form submission | URL change or success message |
| Error state | `expect(page.getByRole('alert')).toBeVisible()` |
| List content | `expect(locator).toHaveCount(n)` or `toContainText(...)` |
| Auth guard | Redirect to /login |

## Step 5 — Create a .env.test file

Create `.env.test` (and add to `.gitignore`) with placeholder credentials:

```
BASE_URL=http://localhost:3000
TEST_EMAIL=test@example.com
TEST_PASSWORD=changeme
```

Tell the user to fill in real test credentials before running.

## Step 6 — Report and offer to run

Summarize what was generated:
- Files created (list with one-line description each)
- Which credentials to set in `.env.test`
- Any routes/features skipped and why

Then **ask the user** whether they want to run the tests now or later:

> "Tests are ready. Do you want me to run them now, or would you prefer to run them yourself?
> If you want me to run them, make sure the dev server is running first (`npm run dev` or equivalent)."

If the user says **yes, run them**:

1. Check Playwright is installed:
   ```bash
   npx playwright --version 2>/dev/null || echo "NOT_INSTALLED"
   ```
   If not installed, run: `npx playwright install --with-deps chromium`

2. Run the tests from the project root:
   ```bash
   cd <project-root> && npx playwright test --reporter=list
   ```

3. Parse the output and report to the user:
   - How many tests passed / failed / skipped
   - For each failure: test name, file, line number, and the error message
   - If all pass: confirm with a clear success message

4. If tests fail, offer to fix them:
   > "X tests failed. Do you want me to try to fix them?"
   If yes, read the failing test file, analyse the error, correct the selector or assertion, and re-run.

If the user says **no, run later**, show the exact commands to run manually:

```bash
cd <project-root>
npx playwright install --with-deps chromium   # first time only
npx playwright test                           # run all tests
npx playwright test --ui                      # interactive mode
npx playwright show-report                    # view HTML report after a run
```

## Notes

- Never hardcode real passwords or API keys — always use environment variables.
- Use `await expect(locator).toBeVisible()` rather than manual waits (`waitForTimeout`).
- Each test must be independent — no shared mutable state between tests.
- If the project has no running dev server, note in the summary that the user must start it before running tests.
- If the codebase uses a component library (shadcn, MUI, etc.), adapt selectors — many components render native roles correctly, but check for custom `data-testid` patterns.
