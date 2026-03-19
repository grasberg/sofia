# Core Template Structure with Optional Modules

## Overview
A flexible product configuration system for digital products that separates core required information from optional modules that can be enabled/disabled as needed.

## Design Philosophy
- **Core**: Essential product information required for any digital product
- **Modules**: Optional functionality that can be added or removed
- **Extensible**: Easy to add new modules without breaking existing configurations
- **Platform-agnostic**: Works with Gumroad, Stripe, or any other platform

## File Structure
```
workspace/products/
├── product.go                 # Go struct definitions
├── product_template.yaml      # Template with all options
├── ai_prompts_product1.yaml   # Example configuration
├── validate.py               # Python validation script
└── CONFIGURATION_README.md   # This file
```

## Core Structure (Required)

### Product
- `core`: Core product information
- `modules`: Optional modules (can be empty)
- `metadata`: Configuration metadata

### Core Fields
| Field | Type | Description | Required |
|-------|------|-------------|----------|
| `name` | string | Product name | Yes |
| `short_description` | string | Brief description for listings | Yes |
| `full_description` | string | Detailed description (Markdown) | Yes |
| `price` | Price | Pricing information | Yes |
| `type` | string | Product type: `digital_download`, `course`, `software`, `template` | Yes |
| `delivery_method` | string | How product is delivered: `direct_download`, `email`, `access_link` | Yes |
| `license` | string | License type: `personal`, `personal_commercial`, `commercial` | Yes |
| `files` | File[] | Product files | Yes |
| `category` | string | Main category | Yes |
| `subcategory` | string | Subcategory | No |
| `tags` | string[] | Search tags | No |

### Price
| Field | Type | Description |
|-------|------|-------------|
| `amount` | float | Base price |
| `currency` | string | Currency code (SEK, USD, EUR) |
| `tiers` | Tier[] | Tiered pricing options |

### File
| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Display name |
| `description` | string | File description |
| `source_path` | string | Path to source file (Markdown, etc.) |
| `output_path` | string | Path to generated file (PDF, etc.) |
| `format` | string | File format: `pdf`, `zip`, `mp4`, etc. |
| `is_main` | bool | Whether this is the main product file |

## Optional Modules

### Module Structure
Each module follows the same pattern:
```yaml
module_name:
  enabled: true/false
  settings:
    # Module-specific settings
```

### Available Modules

#### 1. Bonus Materials
Extra files or content included with the product.

**Settings:**
```yaml
bonus_materials:
  enabled: true
  settings:
    materials:
      - name: "Extra Prompts Pack"
        description: "Additional prompts for advanced users"
        file_path: "dist/pdfs/bonus_extras.pdf"
```

#### 2. Campaigns
Launch discounts and coupon codes.

**Settings:**
```yaml
campaigns:
  enabled: true
  settings:
    launch_discount:
      percentage: 20
      duration_days: 30
    coupon_codes:
      - code: "WELCOME10"
        percentage: 10
        expires: "2026-04-30"
```

#### 3. Affiliate Program
Affiliate marketing settings.

**Settings:**
```yaml
affiliate:
  enabled: true
  settings:
    commission_rate: 7.0  # percentage
    cookie_duration: 30   # days
```

#### 4. Gumroad Integration
Platform-specific settings for Gumroad.

**Settings:**
```yaml
gumroad:
  enabled: true
  settings:
    visibility: "public"
    purchase_type: "fixed_price"
    quantity: "unlimited"
    custom_permalink: ""
    thumbnail: "assets/thumbnail.jpg"
```

#### 5. Build Configuration
Scripts and dependencies for building the product.

**Settings:**
```yaml
build:
  enabled: true
  settings:
    scripts:
      - name: "generate_pdf"
        command: "./build-scripts/generate-pdf.sh"
        inputs: ["source.md"]
        outputs: ["output.pdf"]
    dependencies:
      - pandoc
      - weasyprint
```

#### 6. Testing
Automated tests for the product pipeline.

**Settings:**
```yaml
testing:
  enabled: true
  settings:
    tests:
      - name: "price_validation"
        script: "scripts/test_price.py"
      - name: "file_exists"
        script: "scripts/test_files.py"
```

## Usage Examples

### 1. Basic Product
```yaml
core:
  name: "My Digital Product"
  short_description: "A great digital product"
  full_description: "Detailed description..."
  price:
    amount: 49
    currency: USD
  type: digital_download
  delivery_method: direct_download
  license: personal
  files:
    - name: "Main PDF"
      source_path: "product.md"
      output_path: "product.pdf"
      format: pdf
      is_main: true
  category: "Education"
modules: {}  # No modules enabled
```

### 2. Full Featured Product
See `ai_prompts_product1.yaml` for a complete example with all modules enabled.

## Integration with Deployment Pipeline

This template structure integrates with the Gumroad deployment pipeline:

1. **Configuration**: Product YAML defines what to deploy
2. **Build**: Modules can trigger build scripts
3. **Validation**: Automated validation of configuration
4. **Upload**: Gumroad module provides platform-specific settings
5. **Testing**: Testing module ensures quality

## Extending with New Modules

To add a new module:

1. Add the module struct to `product.go`
2. Update the validation script
3. Document the module in this README
4. Add to the template YAML

Example new module struct:
```go
type NewModuleSettings struct {
    Field1 string `json:"field1" yaml:"field1"`
    Field2 int    `json:"field2" yaml:"field2"`
}
```

## Validation

Run validation with:
```bash
python3 validate.py product.yaml
```

Or using Go (once implemented):
```bash
go run validate.go product.yaml
```

## Next Steps

1. **Implement Go validator** - Create a proper Go package for validation
2. **Create CLI tool** - Command-line tool for managing products
3. **Integrate with Gumroad API** - Use configuration to automate uploads
4. **Add more modules** - Email sequences, upsells, analytics
5. **Create template generator** - Interactive tool for creating new products