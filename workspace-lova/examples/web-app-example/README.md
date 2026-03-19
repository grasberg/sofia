# Web App Example

This example demonstrates how to use the folder structure template for a full-stack web application.

## Project Overview
- **Type**: Full-stack web application (React frontend, Node.js backend)
- **Purpose**: Example SaaS dashboard for analytics
- **Modules used**: Core structure + Web App module

## Structure Explanation

### `/docs/`
- `project-brief.md`: Goals, scope, timeline
- `decisions.md`: Technology choices (React, Node.js, PostgreSQL)
- `references.md`: Useful links and resources

### `/assets/`
- `brand/`: Logo, color palette, typography
- `images/`: Screenshots, product images
- `media/`: Demo videos, tutorials

### `/src/frontend/`
- `components/`: Reusable React components (Button, Card, Navbar)
- `pages/`: Page components (Dashboard, Settings, Login)
- `styles/`: CSS modules, global styles, theme
- `public/`: Static assets (favicon, robots.txt)

### `/src/backend/`
- `api/`: REST API routes (users, analytics, billing)
- `models/`: Database models (User, Subscription, Event)
- `services/`: Business logic (authentication, payment processing)
- `middleware/`: Express middleware (auth, logging, error handling)

### `/tests/`
- `unit/`: Unit tests for individual functions and components
- `integration/`: API integration tests, database tests

### `/config/`
- `development.json`: Local development configuration
- `production.json`: Production environment settings
- `secrets.example.json`: Template for environment variables

### `/deployments/`
- `docker/`: Dockerfile, docker-compose.yml
- `kubernetes/`: K8s manifests for production
- `scripts/`: Deployment and build scripts

## Getting Started

1. Clone this template
2. Update project name in package.json and configuration files
3. Install dependencies: `npm install`
4. Start development: `npm run dev`

## Customization

- For TypeScript: Add `tsconfig.json` in root
- For different backend language: Replace `/src/backend/` structure
- For mobile app: Add `/src/mobile/` directory

---

*This is a template - replace with your actual project details.*