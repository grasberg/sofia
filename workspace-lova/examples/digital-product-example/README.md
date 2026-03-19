# Digital Product Example

This example demonstrates the folder structure for a digital product (online course, template pack, or ebook).

## Product Overview
- **Type**: Online course with templates
- **Name**: "Productivity Mastery Course"
- **Price**: $97
- **Modules used**: Core structure + Digital Product module + Marketing module

## Structure Explanation

### `/content/`
- `lessons/`: Course modules (video files, transcripts, slides)
  - `01-welcome/`
  - `02-foundations/`
  - `03-advanced/`
- `worksheets/`: PDF worksheets, exercises, checklists
- `templates/`: Notion templates, Excel sheets, Google Docs
- `bonuses/`: Extra content (cheat sheets, case studies)

### `/marketing/`
- `sales-page/`: Landing page copy, design assets, A/B test variations
- `email-sequence/`: Email campaigns (welcome, nurture, launch sequences)
- `social-media/`: Social posts, graphics, video clips

### `/delivery/`
- `downloads/`: Zipped product files, individual asset files
- `access-control/`: Customer access logic, coupon codes, license keys
- `updates/`: Version history, update notes, patch files

### `/docs/`
- Product roadmap, customer research, competitor analysis

### `/assets/`
- Product images, demo videos, testimonial graphics

## Workflow

1. **Content Creation**: Add lessons to `/content/lessons/`
2. **Marketing Preparation**: Prepare launch materials in `/marketing/`
3. **Delivery Setup**: Configure access and downloads in `/delivery/`
4. **Launch**: Execute marketing campaign
5. **Updates**: Post-launch content updates in `/content/updates/`

## Tools Integration

- **Email marketing**: ConvertKit/Mailchimp sequences in `/marketing/email-sequence/`
- **Landing page**: Carrd/Webflow design files in `/marketing/sales-page/`
- **File hosting**: Gumroad/Stripe integration for `/delivery/downloads/`
- **Community**: Discord/Community platform setup

## Pricing Strategy

Document your pricing tiers, discounts, and bundle options in `/docs/pricing.md`

---

*This is a template - customize for your specific digital product.*