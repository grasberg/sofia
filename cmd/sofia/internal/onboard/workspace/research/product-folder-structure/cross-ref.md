# Cross-Reference Analysis: Digital Product Folder Structure

**Date:** 2026-03-19

## Common Themes Across Sub-Questions

### 1. Separation of Concerns (Appears in SQ1, SQ3, SQ4)
- **SQ1**: Code vs configuration vs static assets
- **SQ3**: Configuration separate from code (12 Factor App)
- **SQ4**: Data vs docs vs src vs tests separation

**Conclusion:** Digital product structure should clearly separate different concerns: source code, configuration, static assets, documentation, tests, deployments.

### 2. Hierarchy and Organization (Appears in SQ2, SQ4, SQ5)
- **SQ2**: Pyramid structure, avoid deep nesting
- **SQ4**: Broad → specific hierarchy
- **SQ5**: Monorepo vs polyrepo organization patterns

**Conclusion:** Balanced hierarchy with 3-4 levels maximum. Top-level categories should be intuitive.

### 3. Consistency Across Projects (Appears in SQ5, SQ4)
- **SQ5**: Standardized scripts, configurations, folder structure across products
- **SQ4**: Consistent naming conventions

**Conclusion:** Need template that ensures consistency across different product types.

### 4. Asset Management (Appears in SQ2, SQ1)
- **SQ2**: Marketing assets organized by campaign, type, version
- **SQ1**: Public assets in `public/` folder

**Conclusion:** Separate `assets/` folder for product-specific marketing materials, screenshots, videos.

### 5. Configuration Management (Appears in SQ3, SQ1)
- **SQ3**: Environment variables for configuration, `.env` files
- **SQ1**: Config files at root (`package.json`, `next.config.js`)

**Conclusion:** `config/` folder for non-sensitive configuration, `.env` for secrets, environment-specific configs.

## Contradictions and Resolutions

### **Contradiction 1:** Feature-based vs Layer-based organization
- **SQ1**: React community debates feature-based vs layer-based
- **SQ4**: Reddit recommends feature-based (domain-driven)
- **SQ5**: Monorepos often use both (apps vs packages)

**Resolution:** Support both patterns through flexible template. Default to feature-based for business logic, layer-based for shared infrastructure.

### **Contradiction 2:** Monorepo vs Polyrepo
- **SQ5**: Different trade-offs, no one-size-fits-all

**Resolution:** Design structure that works in both contexts. Product template should be usable standalone (polyrepo) or as subdirectory (monorepo).

### **Contradiction 3:** Configuration in environment vs files
- **SQ3**: 12 Factor App says env vars only
- **SQ1**: Framework config files (next.config.js) are common

**Resolution:** Hybrid approach: Sensitive data in env vars, non-sensitive defaults in config files. Provide both `.env.example` and `config/` folder.

## Gaps Identified

1. **Product Metadata**: No standard for product metadata (name, version, description, pricing, etc.)
2. **Launch Materials**: Structure for launch-specific assets (launch checklist, email sequences, ads)
3. **Analytics and Tracking**: Where to store analytics configurations, tracking codes, conversion pixels
4. **Legal and Compliance**: Legal documents (terms, privacy policy, licenses)
5. **Customer Support**: Support materials (FAQ, troubleshooting guides, contact information)

## Synthesis for Template Design

Based on cross-referencing, a digital product folder structure should include:

### Core Directories
1. **`src/`** - Source code (framework-appropriate structure)
2. **`public/`** or `static/` - Static assets
3. **`assets/`** - Product marketing assets (organized by type/campaign)
4. **`config/`** - Configuration files (non-sensitive)
5. **`docs/`** - Documentation
6. **`tests/`** - Test files
7. **`scripts/`** - Build and utility scripts
8. **`deploy/`** - Deployment configurations

### Product-Specific Directories
9. **`product/`** - Product metadata, pricing, descriptions
10. **`launch/`** - Launch materials and sequences
11. **`legal/`** - Legal documents
12. **`analytics/`** - Tracking and analytics config

### Support for Multiple Product Types
- Web app: `src/` follows framework conventions
- Digital download: `content/` folder for downloadable files
- Template product: `templates/` folder
- SaaS: `client/`, `server/`, `shared/` subdirectories