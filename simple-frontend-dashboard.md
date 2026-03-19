# Plan: Simple Frontend Dashboard

## Overview
Build a simple frontend dashboard to display key metrics and data visualizations. The dashboard should be clean, responsive, and easy to extend with new widgets. This will serve as a foundation for various analytics needs (affiliate tracking, e-commerce, automation ROI, etc.) depending on data source.

## Project Type
**WEB** application - primary agent: `frontend-specialist`

## Success Criteria
- [ ] Dashboard loads with 3-5 example widgets (KPI cards, chart, table)
- [ ] Fully responsive design (mobile, tablet, desktop)
- [ ] Mock data fetched from a local JSON file or static API
- [ ] Clean, modern UI with consistent styling
- [ ] Extensible widget system for adding new components
- [ ] No backend required for initial version (static frontend)

## Tech Stack
| Technology | Rationale |
|------------|-----------|
| **React 18** | Component-based, widely used, good ecosystem |
| **TypeScript** | Type safety, better developer experience |
| **Vite** | Fast build tool, hot reload, simple configuration |
| **Tailwind CSS** | Utility-first CSS for rapid UI development |
| **Chart.js** with **react-chartjs-2** | Simple, flexible charts for data visualization |
| **Lucide React** | Lightweight icon library |
| **date-fns** | Date formatting utilities |

## File Structure
```
simple-dashboard/
├── public/
│   ├── index.html
│   └── favicon.svg
├── src/
│   ├── components/
│   │   ├── Layout/
│   │   │   ├── Header.tsx
│   │   │   ├── Sidebar.tsx
│   │   │   └── MainLayout.tsx
│   │   ├── Widgets/
│   │   │   ├── KpiCard.tsx
│   │   │   ├── LineChart.tsx
│   │   │   ├── BarChart.tsx
│   │   │   ├── DataTable.tsx
│   │   │   └── WidgetGrid.tsx
│   │   └── common/
│   │       ├── Button.tsx
│   │       └── Card.tsx
│   ├── hooks/
│   │   └── useMockData.ts
│   ├── types/
│   │   └── dashboard.ts
│   ├── utils/
│   │   ├── formatters.ts
│   │   └── constants.ts
│   ├── data/
│   │   └── mockData.json
│   ├── App.tsx
│   ├── main.tsx
│   └── index.css
├── package.json
├── tsconfig.json
├── vite.config.ts
├── tailwind.config.js
└── README.md
```

## Task Breakdown

### Task 1: Project Setup & Configuration
**Agent:** `frontend-specialist`  
**Skills:** `clean-code`  
**Priority:** P0  
**Dependencies:** None  
**INPUT:** Empty directory  
**OUTPUT:** Vite + React + TypeScript + Tailwind project with basic structure  
**VERIFY:** `npm run dev` starts dev server, shows "Hello World"

### Task 2: Core Layout Components
**Agent:** `frontend-specialist`  
**Skills:** `clean-code`  
**Priority:** P0  
**Dependencies:** Task 1  
**INPUT:** Basic project structure  
**OUTPUT:** Header, Sidebar, MainLayout components with responsive design  
**VERIFY:** Layout adapts to screen size, navigation links placeholder

### Task 3: Widget System Foundation
**Agent:** `frontend-specialist`  
**Skills:** `clean-code`  
**Priority:** P1  
**Dependencies:** Task 2  
**INPUT:** Layout components  
**OUTPUT:** WidgetGrid component with CSS Grid, Card component, KpiCard component  
**VERIFY:** Grid displays 3 KPI cards with mock data

### Task 4: Data Visualization Widgets
**Agent:** `frontend-specialist`  
**Skills:** `clean-code`  
**Priority:** P1  
**Dependencies:** Task 3  
**INPUT:** Widget system  
**OUTPUT:** LineChart and BarChart components using Chart.js with mock data  
**VERIFY:** Charts render with sample datasets, responsive to container size

### Task 5: Data Table Widget
**Agent:** `frontend-specialist`  
**Skills:** `clean-code`  
**Priority:** P2  
**Dependencies:** Task 3  
**INPUT:** Widget system  
**OUTPUT:** DataTable component with sorting, pagination (client-side)  
**VERIFY:** Table displays mock data, sort headers work, pagination controls

### Task 6: Mock Data & Hooks
**Agent:** `frontend-specialist`  
**Skills:** `clean-code`  
**Priority:** P2  
**Dependencies:** Task 1  
**INPUT:** Project structure  
**OUTPUT:** mockData.json with realistic metrics, useMockData hook for data fetching  
**VERIFY:** All widgets use mock data hook, data updates cause re-render

### Task 7: Styling & Polish
**Agent:** `frontend-specialist`  
**Skills:** `clean-code`  
**Priority:** P3  
**Dependencies:** Tasks 2-5  
**INPUT:** All components built  
**OUTPUT:** Consistent color scheme, typography, spacing, hover states  
**VERIFY:** UI looks cohesive, passes WCAG color contrast checks

### Task 8: Responsive Optimization
**Agent:** `frontend-specialist`  
**Skills:** `clean-code`  
**Priority:** P3  
**Dependencies:** Task 7  
**INPUT:** Styled components  
**OUTPUT:** Mobile-first responsive adjustments, touch-friendly interactions  
**VERIFY:** Dashboard works on 320px to 1920px viewports

### Task 9: Build & Deployment Setup
**Agent:** `frontend-specialist`  
**Skills:** `clean-code`  
**Priority:** P3  
**Dependencies:** All previous tasks  
**INPUT:** Complete dashboard  
**OUTPUT:** Production build script, Netlify configuration (netlify.toml)  
**VERIFY:** `npm run build` succeeds, build output in /dist

## Phase X: Verification

### 1. Development Verification
```bash
# P0: Lint & Type Check
npm run lint && npx tsc --noEmit

# P0: Security Scan
python .agent/skills/vulnerability-scanner/scripts/security_scan.py .

# P1: UX Audit
python .agent/skills/frontend-design/scripts/ux_audit.py .

# P3: Lighthouse Audit (requires running server)
python .agent/skills/performance-profiling/scripts/lighthouse_audit.py http://localhost:5173
```

### 2. Build Verification
```bash
npm run build
# → No errors/warnings
```

### 3. Runtime Verification
```bash
npm run dev
# Open browser, test all widgets
```

### 4. Rule Compliance (Manual Check)
- [ ] No purple/violet hex codes
- [ ] No standard template layouts
- [ ] Socratic Gate was respected

### 5. Phase X Completion Marker
```markdown
## ✅ PHASE X COMPLETE
- Lint: ✅ Pass
- Security: ✅ No critical issues
- Build: ✅ Success
- Date: [Current Date]
```

## Risks & Mitigation
| Risk | Mitigation |
|------|------------|
| Over-engineering dashboard widgets | Start with 3-5 essential widgets, keep them simple |
| Chart.js bundle size too large | Use tree-shaking, consider lighter alternative if needed |
| Responsive design complexity | Mobile-first approach, test on multiple viewports early |
| Data source integration postponed | Use mock data with realistic schema for easy migration |

## Notes
- This plan assumes generic dashboard needs; specific data requirements can be added later
- Backend integration is out of scope for "simple frontend dashboard"
- Deployment to Netlify can use the existing deployment-guide knowledge