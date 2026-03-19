# AI Prompts Product - Project Structure Plan

## Overview
Create a structured project folder for the AI prompts product as an example template for how all digital products should be organized. This will serve as a reference structure for future digital product development.

## Project Type
BACKEND (File system organization, no frontend/mobile UI)

## Success Criteria
- Complete folder structure created according to defined template
- All existing files moved to appropriate locations
- Documentation (README.md) explaining the structure
- Structure can be easily copied for future digital products
- Verification that all paths exist and are accessible

## Tech Stack
- File system operations (mkdir, mv, cp)
- Markdown for documentation
- No specific programming language required

## File Structure
```
ai-prompts-product/
├── product/
│   ├── prompts/           # All prompt files (Markdown, text)
│   ├── assets/           # Images, screenshots, media files
│   └── pdf/              # Compiled PDF versions
├── landing-page/
│   ├── assets/           # CSS, JS, fonts
│   └── images/           # Landing page specific images
├── marketing/
│   ├── social-media/     # Posts, graphics for social platforms
│   ├── email/            # Email sequences, templates
│   └── ads/              # Ad copy, targeting info
├── distribution/
│   ├── gumroad/          # Gumroad-specific files, metadata
│   └── other-platforms/  # Other distribution channels
├── docs/
│   ├── checklists/       # Launch checklists, quality assurance
│   └── notes/            # Research, brainstorming notes
└── README.md             # Structure documentation
```

## Task Breakdown

### Task 1: Verify Current Structure
**Agent:** project-planner (self)
**Skills:** None required
**Priority:** P0
**Dependencies:** None
**INPUT:** Current ai-prompts-product directory
**OUTPUT:** Inventory of existing files and folders, mapping to target structure
**VERIFY:** List of current items matches what's found in workspace/ai-prompts-product

### Task 2: Create Missing Directories
**Agent:** backend-specialist
**Skills:** None required  
**Priority:** P0
**Dependencies:** Task 1
**INPUT:** List of missing directories from defined structure (excluding already existing: product/prompts, product/assets, docs/notes)
**OUTPUT:** All missing directories created
**VERIFY:** All directories in file structure exist (check with `ls -la`)

### Task 3: Organize Existing Files
**Agent:** backend-specialist
**Skills:** None required
**Priority:** P1
**Dependencies:** Task 2
**INPUT:** Current files in ai-prompts-product root
**OUTPUT:** Files moved to appropriate subdirectories
**VERIFY:** Root directory contains only README.md and maybe config files; all content files moved

### Task 4: Create Documentation
**Agent:** backend-specialist
**Skills:** None required
**Priority:** P1
**Dependencies:** Task 3
**INPUT:** Completed folder structure
**OUTPUT:** README.md explaining the structure and usage
**VERIFY:** README.md exists and contains clear instructions

### Task 5: Create Example Files
**Agent:** backend-specialist
**Skills:** None required
**Priority:** P2
**Dependencies:** Task 4
**INPUT:** Empty directory structure
**OUTPUT:** Example placeholder files in each directory
**VERIFY:** Each directory contains at least one example file (e.g., .gitkeep or placeholder.txt)

### Task 6: Validate Structure
**Agent:** project-planner (self)
**Skills:** None required
**Priority:** P2
**Dependencies:** Task 5
**INPUT:** Completed project structure
**OUTPUT:** Validation report
**VERIFY:** Structure matches template, all files accessible, documentation complete

## Phase X: Verification

### Mandatory Script Execution
```bash
# Verify directory structure exists
python -c "
import os
expected = ['product/prompts', 'product/assets', 'product/pdf', 'landing-page/assets', 'landing-page/images', 'marketing/social-media', 'marketing/email', 'marketing/ads', 'distribution/gumroad', 'distribution/other-platforms', 'docs/checklists', 'docs/notes']
base = 'workspace/ai-prompts-product'
all_exist = True
for d in expected:
    path = os.path.join(base, d)
    if not os.path.exists(path):
        print(f'Missing: {path}')
        all_exist = False
if all_exist:
    print('All directories exist ✓')
else:
    exit(1)
"

# Verify README exists
test -f workspace/ai-prompts-product/README.md && echo "README exists ✓" || exit 1

# Verify root directory is clean (only expected files)
echo "Checking root directory..."
ls -la workspace/ai-prompts-product/
```

### Build Verification
N/A (No build process for file structure)

### Runtime Verification
N/A (No runtime for file structure)

### Rule Compliance
- [ ] No purple/violet hex codes (N/A)
- [ ] No standard template layouts (N/A)
- [ ] Socratic Gate was respected

### Phase X Completion Marker
## ✅ PHASE X COMPLETE
- Directory structure: ✅ All created
- Files organized: ✅ Moved to correct locations
- Documentation: ✅ README.md exists
- Date: 2026-03-19
