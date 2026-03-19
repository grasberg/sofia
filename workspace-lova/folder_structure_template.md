# Project Folder Structure Template

A flexible, scalable folder structure template for various project types: web apps, SaaS, digital products, consulting projects, and marketing campaigns.

## Philosophy

- **Consistency**: Same structure across projects for easy navigation
- **Modularity**: Core structure + optional modules for specific needs
- **Documentation**: Self-documenting with README files in key folders
- **Scalability**: Works for small projects and grows with complexity

## Core Structure (Mandatory)

Every project should have these base folders:

```
project-name/
├── README.md
├── .gitignore
├── .env.example
├── docs/
│   ├── project-brief.md
│   ├── decisions.md
│   └── references.md
├── assets/
│   ├── brand/
│   ├── images/
│   └── media/
├── src/
│   ├── core/
│   ├── utils/
│   └── main.js|py|go (entry point)
├── tests/
│   ├── unit/
│   └── integration/
├── config/
│   ├── development.json
│   ├── production.json
│   └── secrets.example.json
└── deployments/
    ├── docker/
    ├── kubernetes/
    └── scripts/
```

## Module System (Optional)

Add modules based on project type:

### Web App Module
```
src/
├── frontend/
│   ├── components/
│   ├── pages/
│   ├── styles/
│   └── public/
└── backend/
    ├── api/
    ├── models/
    ├── services/
    └── middleware/
```

### SaaS Module
```
src/
├── auth/
├── billing/
├── multi-tenant/
├── admin/
└── analytics/
deployments/
├── terraform/
├── ci-cd/
└── monitoring/
```

### Digital Product Module
```
content/
├── lessons/
├── worksheets/
├── templates/
└── bonuses/
marketing/
├── sales-page/
├── email-sequence/
└── social-media/
delivery/
├── downloads/
├── access-control/
└── updates/
```

### Consulting/Service Module
```
client/
├── brief/
├── deliverables/
├── communications/
└── invoices/
research/
├── competitive-analysis/
├── interviews/
└── findings/
presentations/
├── proposals/
├── reports/
└── workshops/
```

### Marketing/Launch Module
```
campaigns/
├── pre-launch/
├── launch-day/
└── post-launch/
content/
├── blog/
├── social/
└── email/
analytics/
├── tracking/
├── dashboards/
└── reports/
```

## Detailed Folder Descriptions

### docs/
- `project-brief.md`: Project goals, scope, timeline, stakeholders
- `decisions.md`: Architecture decisions, technology choices, rationale
- `references.md`: Links to research, articles, tools, inspiration

### assets/
- `brand/`: Logo files, color palette, typography, brand guidelines
- `images/`: Product screenshots, marketing images, icons
- `media/`: Videos, audio files, animations

### src/
- `core/`: Business logic, domain models, core algorithms
- `utils/`: Helper functions, utilities, shared code
- Entry point: Main application file

### tests/
- `unit/`: Unit tests for individual components
- `integration/`: Tests for integrated systems
- `e2e/`: End-to-end tests (optional)

### config/
- Environment-specific configuration files
- Never commit secrets; use `.env.example` as template

### deployments/
- Infrastructure as code, deployment scripts, CI/CD configurations

## Project Type Templates

### 1. Web Application (Full-stack)
```
project-name/
├── core-structure/
├── web-app-module/
└── deployments/ (with CI/CD)
```

### 2. Digital Product (Course/Template)
```
project-name/
├── core-structure/
├── digital-product-module/
└── marketing-module/
```

### 3. SaaS Product
```
project-name/
├── core-structure/
├── saas-module/
└── web-app-module/
```

### 4. Consulting Project
```
project-name/
├── core-structure/
├── consulting-module/
└── client/
```

### 5. Marketing Campaign
```
project-name/
├── core-structure/
└── marketing-module/
```

## Quick Start Script

Create a bash script to scaffold new projects:

```bash
#!/bin/bash
# create-project.sh
# Usage: ./create-project.sh <project-name> <type> [modules...]

# Example: ./create-project.sh my-saas saas web-app analytics
```

## Best Practices

1. **Keep it flat**: Avoid nesting deeper than 3-4 levels
2. **Name clearly**: Use descriptive, lowercase names with hyphens
3. **Document each folder**: Add README.md in key folders explaining purpose
4. **Version control**: Include appropriate .gitignore for your stack
5. **Environment separation**: Keep development, staging, production configs separate

## Example Projects

See the `examples/` folder for:
- `web-app-example/`: React + Node.js application
- `digital-product-example/`: Online course with marketing materials
- `saas-example/`: Multi-tenant SaaS with billing
- `consulting-example/`: Client project with deliverables

---

## Customization

Adapt this template to your needs:
1. Start with core structure
2. Add relevant modules
3. Remove unnecessary folders
4. Add team-specific conventions

## Contributing

Found a better structure? Submit improvements via pull request.

---

*Template version: 1.0*
*Last updated: March 2026*
