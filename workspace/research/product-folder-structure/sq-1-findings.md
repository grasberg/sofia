# Sub-Question 1 Findings: Web Application Structure

**Source 1:** Next.js Documentation - Project Structure
**URL:** https://nextjs.org/docs/app/getting-started/project-structure
**Accessed:** 2026-03-19

## Key Findings

### Next.js Recommended Structure
- Next.js supports storing application code inside an optional `src` folder
- This separates application code from project configuration files which mostly live in the root
- Typical root-level files: `package.json`, `next.config.js`, `tsconfig.json`, `.gitignore`
- Inside `src/`: `app/` (App Router), `components/`, `lib/`, `styles/`, `utils/`
- The `public/` folder (root level) for static assets

### React Folder Structure (Robin Wieruch)
**URL:** https://www.robinwieruch.de/react-folder-structure/
**Accessed:** 2026-03-19

- Most React projects start with a `src/` folder and one `src/[name].(js|ts|jsx|tsx)` file
- Using Vite for client-side React: `src/App.jsx`
- Using Next.js for server-driven React: `src/app/page.js`
- Common patterns:
  - Feature-based organization: group by feature/module
  - Layer-based organization: separate by technical concern (components, hooks, utils, etc.)
  - Hybrid approaches

### React Legacy Documentation
**URL:** https://legacy.reactjs.org/docs/faq-structure.html
**Accessed:** 2026-03-19

- Two popular approaches:
  1. Group by file type: `components/`, `utils/`, `api/`, `styles/`
  2. Group by feature/module: `user/`, `product/`, `dashboard/`
- Suggestion: Ask users of your product what major parts it consists of, use their mental model as blueprint

### Popular React Folder Structures (Profy.dev)
**URL:** https://profy.dev/article/react-folder-structure
**Accessed:** 2026-03-19

- Evolution of folder structures in growing codebases
- Problems with flat structures as projects scale
- Feature-based folder structure recommended for maintainability
- Screaming Architecture: structure should scream about what the system does, not about frameworks

## Patterns Identified

1. **Separation of concerns**: Code vs configuration vs static assets
2. **`src/` convention**: Source code inside `src/` folder, config files at root
3. **Public assets**: `public/` folder at root for static files
4. **Organization approaches**: Feature-based vs layer-based
5. **Framework-specific conventions**: Next.js has `app/` or `pages/` router structure

## Implications for Digital Product Structure
- Web apps should follow framework conventions for easier onboarding
- Need to accommodate both `src/`-based and root-based structures
- Consider separation of product-specific code from shared infrastructure