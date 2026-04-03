# Sub-Question 4 Findings: Industry Standards

**Source 1:** World Bank Template - Folder Structure and Naming Conventions
**URL:** https://worldbank.github.io/template/docs/folders-and-naming.html
**Accessed:** 2026-03-19

## Key Findings

### World Bank Project Template Structure
```
project/
├── data/
│   ├── raw/          # Original, immutable data
│   ├── processed/    # Cleaned, transformed data
│   └── outputs/      # Analysis outputs, figures, tables
├── docs/             # Documentation
├── src/              # Source code
│   ├── analysis/     # Analysis scripts
│   └── utils/        # Utility functions
├── tests/            # Test files
├── reports/          # Generated reports
└── README.md         # Project overview
```

### MIT Broad Institute File Structure Guide
**URL:** https://mitcommlab.mit.edu/broad/commkit/file-structure/
**Accessed:** 2026-03-19

- Separate concerns into a hierarchy of folders
- Use consistent, chronological, and descriptive names
- Common top-level categories:
  - `Administrative/` (proposals, budgets, timelines)
  - `Research/` (data, analysis, literature)
  - `Communication/` (presentations, papers, graphics)
  - `Archival/` (old versions, backups)

### Pyramid Folder Structure (Extensis)
**URL:** https://www.extensis.com/blog/how-to-create-a-manageable-and-logical-folder-structure
**Accessed:** 2026-03-19

- Pyramid structure: broad categories at top, specific details deeper
- Start with general overview folder, then drill down
- Avoid too many folders at the same level (cognitive overload)
- Balance breadth vs depth

### Code Standards and Folder Structure (TutorialsPoint)
**URL:** https://www.tutorialspoint.com/code-standards-and-folder-structure-in-a-project
**Accessed:** 2026-03-19

- Five categories of standards:
  1. Readability
  2. Maintainability
  3. Reusability
  4. Testability
  5. Scalability
- Folder structure should support these qualities

### Reddit /r/dotnet Discussion
**URL:** https://www.reddit.com/r/dotnet/comments/170kb2o/naming_conventions_for_files_folders_is_there_a/
**Accessed:** 2026-03-19

- Organize by feature/domain, not by technical details
- Example: `Pizza/` folder containing `PizzaController.cs`, `PizzaService.cs`, `PizzaModel.cs`
- Feature-based organization improves cohesion

## Industry Patterns Identified

1. **Separation by concern**: Data, docs, src, tests, reports
2. **Data pipeline structure**: `raw/` → `processed/` → `outputs/`
3. **Feature-based organization**: Group related files by feature/domain
4. **Pyramid hierarchy**: Broad to specific, avoid deep nesting
5. **Consistent naming**: Descriptive, chronological, lowercase-with-hyphens

## Widely Adopted Conventions

1. **`src/`**: Source code
2. **`tests/`** or `__tests__/`: Test files
3. **`docs/`** or `documentation/`: Documentation
4. **`public/`** or `static/`: Static assets
5. **`config/`** or `configuration/`: Configuration files
6. **`scripts/`**: Build and utility scripts
7. **`dist/`** or `build/`: Build outputs
8. **`node_modules/`**, `vendor/`: Dependencies (usually excluded)

## Implications for Digital Product Structure
- Should align with industry conventions for familiarity
- Need flexibility for different project types (data science vs web app vs digital product)
- Consider feature-based vs layer-based organization
- Include standard directories like `docs/`, `tests/`, `config/`