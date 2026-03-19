# Plan: Standardized Product Folder Structure

## Overview
Design a standardized folder structure for digital products that can accommodate different product types (digital downloads, SaaS, web apps) with consistent organization for specs, pricing, usage guides, marketing materials, technical implementation, distribution, legal, and analytics.

**Why this matters:** Currently product specifications are scattered across multiple files in `workspace/products/` without a clear structure. A standardized template will improve organization, streamline creation of new products, and enable automation in deployment and marketing.

## Project Type
**WEB** (documentation/organization system, not a web application)
- Primary Agent: `frontend-specialist` (for documentation site if needed)
- Note: This is primarily a file structure design project, not a code implementation project.

## Success Criteria
1. **Consistent Structure:** All future digital products follow the same folder hierarchy
2. **Complete Coverage:** Structure includes all necessary components: product specs, pricing tiers, usage guides, marketing assets, technical code (if applicable), deployment configs, legal documents, analytics tracking
3. **Flexibility:** Structure adapts to different product types (digital downloads vs SaaS vs web apps)
4. **Documentation:** Clear README.md in each folder explaining purpose and contents
5. **Validation:** Applied to at least one existing product (e.g., AI Prompts product) as proof of concept

## Tech Stack
- **Documentation:** Markdown (.md files)
- **Configuration:** JSON/YAML for product metadata
- **Version Control:** Git
- **Optional:** Static site generator (Next.js/MkDocs) for product documentation site

## File Structure
```
products/
├── PRODUCT-NAME/
│   ├── 01-specs/
│   │   ├── product-spec.md          # Core product description
│   │   ├── pricing-tiers.md         # Tier definitions & pricing
│   │   ├── features-list.md         # Feature breakdown
│   │   └── target-audience.md       # Ideal customer profile
│   ├── 02-content/
│   │   ├── how-to-use-guide.md      # User instructions
│   │   ├── tutorials/               # Step-by-step tutorials
│   │   ├── faq.md                   # Frequently asked questions
│   │   └── examples/                # Example outputs/use cases
│   ├── 03-marketing/
│   │   ├── marketing-plan.md        # Go-to-market strategy
│   │   ├── copy/                    # Sales copy variations
│   │   ├── emails/                  # Email sequences
│   │   ├── social-media/            # Social posts
│   │   └── ads/                     # Ad creatives & copy
│   ├── 04-assets/
│   │   ├── images/                  # Product screenshots, logos
│   │   ├── videos/                  # Demo videos, tutorials
│   │   ├── audio/                   # Podcast clips, voiceovers
│   │   └── documents/               # PDFs, printables
│   ├── 05-src/                      # Technical implementation (if applicable)
│   │   ├── web-app/                 # Next.js/React app
│   │   ├── api/                     # Backend API
│   │   ├── scripts/                 # Automation scripts
│   │   └── config/                  # Configuration files
│   ├── 06-deployment/
│   │   ├── staging/                 # Staging environment config
│   │   ├── production/              # Production deployment
│   │   ├── monitoring/              # Monitoring & alerts
│   │   └── backups/                 # Backup procedures
│   ├── 07-documentation/
│   │   ├── api-reference.md         # API documentation
│   │   ├── technical-guide.md       # Technical setup guide
│   │   ├── changelog.md             # Version history
│   │   └── contributing.md          # Contribution guidelines
│   ├── 08-legal/
│   │   ├── terms-of-service.md
│   │   ├── privacy-policy.md
│   │   ├── refund-policy.md
│   │   └── license-agreement.md
│   ├── 09-analytics/
│   │   ├── metrics-dashboard.md     # Key metrics to track
│   │   ├── conversion-funnels.md    # Conversion tracking
│   │   └── customer-feedback.md     # Feedback collection
│   ├── 10-operations/
│   │   ├── support/                 # Support scripts & templates
│   │   ├── billing/                 # Billing configuration
│   │   ├── fulfillment/             # Product delivery automation
│   │   └── updates/                 # Update release process
│   └── README.md                    # Product overview & quick start
└── TEMPLATE/                        # Template for new products (copy this)
```

## Task Breakdown

### Phase 1: Foundation & Design
**P0 Priority**

| Task ID | Name | Agent | Skills | Priority | Dependencies | INPUT → OUTPUT → VERIFY |
|---------|------|-------|--------|----------|--------------|-------------------------|
| T1 | Finalize folder structure design | project-planner | plan-writing | P0 | None | **INPUT:** Research from plan-9, existing product files. **OUTPUT:** Approved folder structure diagram. **VERIFY:** User confirms structure meets all product types. |
| T2 | Create template folder with README files | backend-specialist | clean-code | P0 | T1 | **INPUT:** Approved structure. **OUTPUT:** `products/TEMPLATE/` with all folders and placeholder README.md files. **VERIFY:** All folders exist with basic documentation. |
| T3 | Define product metadata schema | database-architect | clean-code | P0 | T1 | **INPUT:** Product specs analysis. **OUTPUT:** `product-metadata.yaml` schema for standardized product info. **VERIFY:** Schema validates with example products. |

### Phase 2: Documentation & Guidelines
**P1 Priority**

| Task ID | Name | Agent | Skills | Priority | Dependencies | INPUT → OUTPUT → VERIFY |
|---------|------|-------|--------|----------|--------------|-------------------------|
| T4 | Create comprehensive documentation | frontend-specialist | app-builder | P1 | T2 | **INPUT:** Template folder. **OUTPUT:** `PRODUCT_STRUCTURE_GUIDE.md` with usage instructions and examples. **VERIFY:** Documentation covers all folder purposes and usage scenarios. |
| T5 | Create migration guide for existing products | backend-specialist | clean-code | P1 | T2, T4 | **INPUT:** Existing products in `workspace/products/`. **OUTPUT:** `MIGRATION_GUIDE.md` with steps to reorganize. **VERIFY:** Guide provides clear steps for each existing product type. |

### Phase 3: Implementation & Validation
**P2 Priority**

| Task ID | Name | Agent | Skills | Priority | Dependencies | INPUT → OUTPUT → VERIFY |
|---------|------|-------|--------|----------|--------------|-------------------------|
| T6 | Migrate AI Prompts product (example) | backend-specialist | clean-code | P2 | T2, T5 | **INPUT:** AI Prompts product files. **OUTPUT:** Reorganized `products/ai-prompts-vault/` following new structure. **VERIFY:** All existing content properly categorized, no data loss. |
| T7 | Create automation scripts for new products | backend-specialist | clean-code | P2 | T2 | **INPUT:** Template folder. **OUTPUT:** `scripts/create-new-product.sh` that scaffolds structure. **VERIFY:** Script creates complete product folder with placeholders. |
| T8 | Integrate with existing deployment processes | devops-engineer | app-builder | P2 | T6 | **INPUT:** Migration example. **OUTPUT:** Updated deployment docs referencing new structure. **VERIFY:** Deployment scripts work with new organization. |

### Phase 4: Review & Optimization
**P3 Priority**

| Task ID | Name | Agent | Skills | Priority | Dependencies | INPUT → OUTPUT → VERIFY |
|---------|------|-------|--------|----------|--------------|-------------------------|
| T9 | Conduct user workflow review | frontend-specialist | brainstorming | P3 | T6, T7 | **INPUT:** Migrated product and scripts. **OUTPUT:** Usability report with improvement suggestions. **VERIFY:** Report identifies pain points and proposed solutions. |
| T10 | Create video walkthrough | frontend-specialist | app-builder | P3 | T4, T6 | **INPUT:** Documentation and example. **OUTPUT:** 5-minute screen recording demonstrating structure usage. **VERIFY:** Video clearly explains key concepts. |

## Phase X: Verification
**MANDATORY final validation before project completion**

### Checklist
- [ ] **Security Scan:** `python .agent/skills/vulnerability-scanner/scripts/security_scan.py .` (no critical issues)
- [ ] **Template Validation:** Verify no purple/violet hex codes used in documentation
- [ ] **Structure Consistency:** All folders follow naming convention (NN-name, lowercase)
- [ ] **Documentation Complete:** Each folder has README.md with purpose and examples
- [ ] **Migration Test:** AI Prompts product successfully reorganized
- [ ] **Script Functionality:** `create-new-product.sh` works without errors
- [ ] **User Acceptance:** User confirms structure meets requirements

### Script Execution Order
1. **P0:** Security scan
2. **P1:** Template validation (manual)
3. **P2:** Run migration test script
4. **P3:** Verify script functionality
5. **P4:** Final user sign-off

### Completion Marker
```
## ✅ PHASE X COMPLETE
- Date: [Current Date]
- Structure: ✅ Implemented and validated
- Documentation: ✅ Complete
- Example Migration: ✅ Successful
- User Approved: ✅ Yes
```

## Risks & Mitigations
1. **Risk:** Over-engineering structure for simple products.
   **Mitigation:** Keep optional folders clearly marked; provide minimal viable template.
2. **Risk:** Migration of existing products loses file context.
   **Mitigation:** Create mapping documentation before migration; backup original files.
3. **Risk:** Team adoption resistance due to complexity.
   **Mitigation:** Provide clear benefits documentation; start with one product as example.
4. **Risk:** Structure doesn't accommodate future product types.
   **Mitigation:** Design extensible schema; include "custom" folder for edge cases.

## Notes
- This plan builds upon work already completed in plan-9 (research and initial design)
- Focus on practical implementation rather than theoretical perfection
- Priority is creating a working system that can evolve based on real usage