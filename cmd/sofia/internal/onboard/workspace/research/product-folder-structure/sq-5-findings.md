# Sub-Question 5 Findings: Multi-Product Consistency

**Source 1:** Monorepo vs Polyrepo (GitHub)
**URL:** https://github.com/joelparkerhenderson/monorepo-vs-polyrepo
**Accessed:** 2026-03-19

## Key Findings

### Monorepo Characteristics
- Single repository containing multiple projects
- Shared dependencies, configurations, and tooling
- Easier code sharing and refactoring across projects
- Simplified dependency management
- Single version control history
- Examples: Google, Facebook, Twitter

### Polyrepo Characteristics
- Multiple separate repositories (one per project)
- Independent versioning and release cycles
- Clear boundaries between projects
- Easier permission management
- Reduced coupling
- Examples: Most open-source projects

### Earthly Blog - Monorepo vs Polyrepo
**URL:** https://earthly.dev/blog/monorepo-vs-polyrepo/
**Accessed:** 2026-03-19

- Monorepo layout with JavaScript: each project gets its own `package.json`
- Tools like Lerna help manage monorepos
- Language influences choice: Go monorepos common, JavaScript polyrepos common
- Polyrepos more popular for open-source and independent projects

### Buildkite - Choosing Between Monorepo and Polyrepo
**URL:** https://buildkite.com/resources/blog/monorepo-polyrepo-choosing/
**Accessed:** 2026-03-19

- Monorepo: Contains multiple projects, libraries, dependencies in one place
- Polyrepo: Separate repositories for each project
- Factors to consider:
  - Team size and structure
  - Project coupling and dependencies
  - Build and deployment complexity
  - Tooling support

### Monorepo Structure Patterns

1. **Flat structure**:
   ```
   monorepo/
   ├── app-web/
   ├── app-mobile/
   ├── lib-shared/
   └── packages/
   ```

2. **Apps and packages**:
   ```
   monorepo/
   ├── apps/
   │   ├── web-app/
   │   └── mobile-app/
   ├── packages/
   │   ├── ui/
   │   ├── utils/
   │   └── api-client/
   └── tooling/
   ```

3. **Product-based**:
   ```
   monorepo/
   ├── product-a/
   │   ├── frontend/
   │   ├── backend/
   │   └── shared/
   ├── product-b/
   └── shared-infra/
   ```

## Patterns for Multi-Product Consistency

1. **Shared configuration**: Common `eslint`, `prettier`, `typescript` configs
2. **Standardized scripts**: Same `npm run dev`, `npm run build`, `npm run test` across projects
3. **Consistent documentation**: Same README structure, contribution guidelines
4. **Uniform folder structure**: Same top-level folders (`src/`, `tests/`, `docs/`) across projects
5. **Centralized tooling**: Shared CI/CD, Docker, deployment configurations

## Implications for Digital Product Structure

- Need to support both monorepo and polyrepo approaches
- Standardized product template that can be used for individual products
- Shared assets and configurations across products
- Product-specific vs shared components separation
- Consider `products/` folder containing each product as subdirectory
- Allow for product variants (e.g., `product/web-app/`, `product/mobile-app/`)