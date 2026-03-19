# Dashboard Controller - Project Plan

## Overview
Create a structured DashboardController with methods for different dashboard views in the Sofia web UI. The controller will organize dashboard-related API endpoints and provide clean separation of concerns from the main Server struct.

**Why this matters:**
- Centralizes dashboard logic for better maintainability
- Provides clear API for frontend dashboard views
- Follows Go best practices with controller pattern
- Makes it easier to add new dashboard features

## Project Type
**BACKEND** - Go controller for web API endpoints

## Success Criteria
- [ ] DashboardController struct exists in `pkg/web/dashboard_controller.go`
- [ ] Controller has methods for at least 5 different views: Overview, Agents, Activity, Statistics, Goals
- [ ] All methods return JSON responses
- [ ] Routes registered in `Server` struct (`/api/dashboard/overview`, etc.)
- [ ] Existing dashboard functionality preserved
- [ ] Code follows existing project patterns and conventions

## Tech Stack
| Technology | Purpose | Rationale |
|------------|---------|-----------|
| Go 1.25+ | Implementation | Project's main language |
| net/http | HTTP handling | Standard library, already used |
| JSON | Data format | Consistent with existing APIs |
| AgentLoop | Data source | Access to agent state and metrics |

## File Structure
```
pkg/web/
├── dashboard_controller.go     # New controller
├── dashboard_controller_test.go # Tests
├── server.go                   # Updated with new routes
└── templates/                  # Existing templates
```

## Task Breakdown

### Task 1: Analyze Existing Code Structure
**Agent:** `backend-specialist`  
**Skills:** `clean-code`  
**Priority:** P0  
**Dependencies:** None  
**INPUT:** Current web/server.go, dashboard.go, and related files  
**OUTPUT:** Analysis of dependencies needed for dashboard (agentLoop, config, auditLogger, cronService, etc.)  
**VERIFY:** Documented list of required fields and interfaces

### Task 2: Design DashboardController Struct
**Agent:** `backend-specialist`  
**Skills:** `clean-code`  
**Priority:** P1  
**Dependencies:** Task 1  
**INPUT:** Dependency analysis  
**OUTPUT:** DashboardController struct definition with proper fields and constructor function  
**VERIFY:** Struct compiles without errors, follows project naming conventions

### Task 3: Implement Controller Methods
**Agent:** `backend-specialist`  
**Skills:** `clean-code`  
**Priority:** P1  
**Dependencies:** Task 2  
**INPUT:** DashboardController struct  
**OUTPUT:** 5+ controller methods:
1. `Overview()` - Overall dashboard statistics (active agents, tool calls, errors, etc.)
2. `Agents()` - Detailed agent status and metrics  
3. `Activity()` - Recent activity feed (similar to monitor feed)
4. `Statistics()` - Time-based statistics (tool usage, agent activity over time)
5. `Goals()` - Goal progress and status
**VERIFY:** Each method returns proper JSON structure, handles errors gracefully

### Task 4: Create API Endpoints in Server
**Agent:** `backend-specialist`  
**Skills:** `clean-code`  
**Priority:** P2  
**Dependencies:** Task 3  
**INPUT:** Working DashboardController  
**OUTPUT:** Updated web/server.go with new routes:
- `GET /api/dashboard/overview`
- `GET /api/dashboard/agents`  
- `GET /api/dashboard/activity`
- `GET /api/dashboard/statistics`
- `GET /api/dashboard/goals`
**VERIFY:** Routes register correctly, Server compiles without errors

### Task 5: Test Endpoints
**Agent:** `test-engineer`  
**Skills:** `clean-code`  
**Priority:** P2  
**Dependencies:** Task 4  
**INPUT:** Running Sofia instance with Web UI  
**OUTPUT:** Verified API endpoints return expected JSON data  
**VERIFY:** All endpoints respond with 200 OK, JSON structure matches expectations

### Task 6: Update Documentation
**Agent:** `backend-specialist`  
**Skills:** `plan-writing`  
**Priority:** P3  
**Dependencies:** Task 5  
**INPUT:** Working dashboard controller  
**OUTPUT:** Updated API documentation in relevant docs  
**VERIFY:** Documentation exists and accurately describes new endpoints

## Phase X: Verification Checklist

### P0: Security & Code Quality
- [ ] No hardcoded secrets or API keys
- [ ] Input validation where applicable
- [ ] Error handling prevents information leakage

### P1: Functional Testing
- [ ] All 5 endpoints return valid JSON
- [ ] Overview endpoint includes required statistics
- [ ] Agents endpoint shows agent status
- [ ] Activity endpoint returns recent events
- [ ] Statistics endpoint provides time-based data
- [ ] Goals endpoint shows goal progress

### P2: Integration Testing
- [ ] Endpoints work with existing Web UI
- [ ] No breaking changes to existing functionality
- [ ] Monitor page still works correctly

### P3: Code Quality
- [ ] Follows existing Go conventions in project
- [ ] Proper error handling and logging
- [ ] No race conditions in concurrent access
- [ ] Comments where complex logic exists

### P4: Performance
- [ ] Endpoints respond within 100ms for typical loads
- [ ] No unnecessary data fetching or processing
- [ ] Efficient data structures for dashboard queries

### Final Sign-off
- [ ] All above checks pass
- [ ] Code reviewed and merged
- [ ] User has reviewed and approved
- [ ] Date: 2026-03-19

---
**Plan Created:** 2026-03-19  
**Project Type:** BACKEND  
**Primary Agent:** `backend-specialist`  
**Estimated Time:** 2-3 hours  
**Risk Areas:** Access to required data sources, potential performance impact of new queries