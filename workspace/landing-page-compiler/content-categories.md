# Landing Page Content Categories and Classes

## Overview
Different types of landing pages serve different purposes and require different combinations of components. This document categorizes common landing page types based on their primary goal and content structure.

## Primary Categories

### 1. Product Launch Page
**Purpose:** Sell a specific digital product (SaaS, template, course, ebook)
**Primary Goal:** Direct purchase conversion
**Typical Audience:** End-users seeking solution to a problem
**Common Components:**
1. Hero with strong value proposition
2. Features/benefits showcasing product capabilities
3. Pricing plans with clear CTAs
4. Testimonials/social proof
5. FAQ addressing purchase concerns
6. Guarantee/money-back reassurance

**Examples:**
- Digital template marketplace (Notion templates, Canva templates)
- SaaS subscription page
- Online course sales page
- Ebook/PDF sales page

### 2. Lead Generation Page
**Purpose:** Collect email addresses in exchange for valuable content
**Primary Goal:** Email capture
**Typical Audience:** Potential customers in awareness/consideration stage
**Common Components:**
1. Hero focusing on the lead magnet value
2. Clear form for email capture (primary CTA)
3. Detailed description of the lead magnet content
4. Social proof (number of downloads, testimonials)
5. Privacy reassurance (no spam, unsubscribe anytime)
6. Optional: preview of the content

**Examples:**
- Ebook/checklist download page
- Webinar registration page
- Free trial signup page
- Newsletter subscription page

### 3. Affiliate Program Page
**Purpose:** Recruit affiliates/partners to promote products
**Primary Goal:** Affiliate signups
**Typical Audience:** Influencers, marketers, content creators
**Common Components:**
1. Hero explaining the affiliate opportunity
2. Commission structure (rates, payment terms)
3. Benefits for affiliates (tools, resources, support)
4. Success stories/top earners
5. Registration form/signup process
6. FAQ about program details
7. Resources for affiliates (banners, links, marketing materials)

**Examples:**
- SaaS affiliate program page
- Digital product affiliate program
- Physical product affiliate program

### 4. Informational/Cheatsheet Page
**Purpose:** Share valuable knowledge with optional download
**Primary Goal:** Provide value, build authority, optional lead capture
**Typical Audience:** People seeking specific information or tools
**Common Components:**
1. Hero presenting the informational value
2. Detailed content organized in sections
3. Download CTA for PDF/cheatsheet version
4. Optional: email capture for enhanced version
5. Related resources/links
6. Author bio/credibility indicators

**Examples:**
- AI tools cheatsheet page
- Programming reference sheet
- Industry statistics page
- How-to guide with downloadable checklist

### 5. Service/Consulting Page
**Purpose:** Sell professional services or consulting
**Primary Goal:** Service inquiries/bookings
**Typical Audience:** Businesses/individuals needing expertise
**Common Components:**
1. Hero establishing expertise and value
2. Services offered with detailed descriptions
3. Case studies/portfolio
4. Client testimonials
5. Process/approach explanation
6. Pricing/packages (hourly, project-based)
7. Contact form/booking calendar
8. Credentials/certifications

**Examples:**
- Freelancer/consultant service page
- Agency services page
- Coaching/mentorship page

### 6. Event Registration Page
**Purpose:** Promote and register attendees for events
**Primary Goal:** Ticket sales/registrations
**Typical Audience:** Potential attendees
**Common Components:**
1. Hero with event details (date, location, theme)
2. Event agenda/schedule
3. Speaker bios
4. Ticket tiers/pricing
5. Registration form
6. Venue information
7. Past event highlights

## Content Classes (Structural Elements)

Beyond categories, landing pages consist of reusable content classes:

### A. Persuasive Elements
- **Value Propositions:** Clear statements of benefit
- **Unique Selling Points (USPs):** Differentiators from competitors
- **Pain Point Solutions:** Addressing specific problems
- **Social Proof:** Testimonials, ratings, client logos
- **Authority Indicators:** Credentials, media mentions, certifications

### B. Informational Elements
- **Feature Lists:** Product capabilities
- **Benefit Explanations:** How features help users
- **Process Flows:** Step-by-step explanations
- **FAQ Sections:** Common questions and answers
- **Comparison Tables:** Versus competitors or plans

### C. Interactive Elements
- **CTAs (Call-to-Actions):** Buttons, forms, links
- **Lead Capture Forms:** Email, name, other fields
- **Interactive Demos:** Product previews
- **Calculators/Estimators:** Value calculators
- **Navigation Elements:** Menus, anchors, breadcrumbs

### D. Visual Elements
- **Hero Images/Videos:** Primary visual attention-grabbers
- **Product Screenshots:** Demonstrations of interface
- **Infographics:** Visual data representations
- **Icons/Illustrations:** Supporting visual cues
- **Logos/Badges:** Trust and brand indicators

## Component Mapping by Category

| Component          | Product Launch | Lead Gen | Affiliate | Info/Cheatsheet | Service |
|--------------------|----------------|----------|-----------|-----------------|---------|
| Hero              | ✓              | ✓        | ✓         | ✓               | ✓       |
| Features          | ✓              | ⚠        | ⚠         | ⚠               | ✓       |
| Pricing           | ✓              | ✗        | ⚠         | ✗               | ✓       |
| Testimonials      | ✓              | ✓        | ✓         | ⚠               | ✓       |
| FAQ               | ✓              | ⚠        | ✓         | ⚠               | ⚠       |
| Lead Form         | ⚠              | ✓        | ✓         | ⚠               | ✓       |
| Process/Steps     | ⚠              | ✗        | ⚠         | ✓               | ✓       |
| Content Sections  | ✗              | ✗        | ✗         | ✓               | ⚠       |
| Registration Form | ✗              | ✗        | ✓         | ✗               | ✗       |
| Resources         | ✗              | ✗        | ✓         | ✓               | ✗       |

✓ = Essential, ⚠ = Optional/Contextual, ✗ = Rarely Used

## Configuration Templates by Category

Each category should have a base configuration template that includes the most commonly used components with appropriate defaults.

### Product Launch Template
```yaml
page_type: product_launch
sections:
  - type: header
  - type: hero
    variant: product_hero
  - type: features
    columns: 3
  - type: testimonials
  - type: pricing
    show_toggle: true
  - type: faq
  - type: footer
```

### Lead Generation Template
```yaml
page_type: lead_generation
sections:
  - type: header
  - type: hero
    variant: lead_hero
  - type: lead_form
    position: inline
  - type: content_preview
  - type: testimonials
  - type: footer
```

### Affiliate Program Template
```yaml
page_type: affiliate_program
sections:
  - type: header
  - type: hero
    variant: affiliate_hero
  - type: commission_structure
  - type: benefits
  - type: success_stories
  - type: registration_form
  - type: resources
  - type: faq
  - type: footer
```

## Implementation Notes

1. **Flexibility:** The compiler should allow mixing and matching components regardless of category.
2. **Defaults:** Each component should have sensible defaults for its category.
3. **Validation:** Validate required fields based on page type.
4. **Custom Components:** Allow custom HTML snippets for unique needs.
5. **Progressive Enhancement:** Start with core components, add specialized ones as needed.

## Next Steps

1. Create HTML/CSS/JS templates for each component
2. Define JSON schema for configuration validation
3. Implement category-specific template generators
4. Create example configurations for each category
5. Build the compiler with category-aware defaults