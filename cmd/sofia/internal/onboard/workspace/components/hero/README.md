# Hero Component - Niche Selection Toolkit

## Overview
A brutalist, high-impact hero section designed for the Niche Selection Toolkit landing page. Features bold typography, social proof stats, and an interactive niche scoring visualizer.

## Design Philosophy
- **Brutalist minimalism**: Sharp edges, high contrast, data-driven aesthetic
- **Color scheme**: Black + Lime Green (#84cc16) for maximum impact
- **Typography**: Heavy weights (font-black) with tight tracking
- **Interactive elements**: Brutalist border effects with hover animations

## HTML Structure
```html
<section class="py-12 md:py-24 lg:py-32">
  <!-- Badge -->
  <!-- Main Headline -->
  <!-- Subheadline -->
  <!-- CTA Buttons -->
  <!-- Stats / Social Proof -->
  <!-- Visual Element - Data Visualization -->
</section>
```

## Key Features
1. **Responsive Design**: Works on mobile, tablet, and desktop
2. **Brutalist Borders**: Custom CSS classes for 3D border effects
3. **Interactive Buttons**: Hover and click animations
4. **Social Proof**: Stats grid for credibility
5. **Visual Demo**: Mock niche scoring visualizer

## Customization
- **Colors**: Modify `--color-accent` and `--color-primary` in base.html
- **Content**: Update headlines, stats, and CTA text
- **Layout**: Adjust padding with `py-*` and `px-*` classes

## Dependencies
- Tailwind CSS (via CDN)
- Google Fonts: Inter
- Shared base.html styles

## Usage
1. Include the base.html styles in your page
2. Copy the hero.html content into your layout
3. Ensure buttons have the `btn-brutal` class for interactive effects

## Browser Support
- Modern browsers (Chrome, Firefox, Safari, Edge)
- Mobile responsive down to 320px width