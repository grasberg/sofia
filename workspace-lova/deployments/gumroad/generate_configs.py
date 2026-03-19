#!/usr/bin/env python3
"""
Generate Gumroad configuration files from product registry.

This script reads the product registry (product_registry.json) and generates
individual Gumroad configuration files for each product.

Usage:
    python3 generate_configs.py --registry product_registry.json --output-dir configs/
"""

import json
import os
import argparse
from pathlib import Path
import shutil

def load_registry(registry_path: str) -> dict:
    """Load product registry JSON file."""
    with open(registry_path, 'r', encoding='utf-8') as f:
        return json.load(f)

def generate_gumroad_config(product_info: dict, template_path: str = None) -> dict:
    """
    Generate a Gumroad configuration from product info.
    
    Args:
        product_info: Product information from registry
        template_path: Optional path to template config (not used yet)
    
    Returns:
        Gumroad configuration dictionary
    """
    # Base configuration structure
    config = {
        "product": {
            "name": product_info["name"],
            "description": product_info["description"],
            "price_cents": product_info["price_cents"],
            "currency": product_info["currency"],
            "category": product_info["category"],
            "tags": product_info["tags"],
            "visibility": "public",
            "quantity_available": "unlimited",
            "purchase_type": "pay_what_you_want",
            "custom_fields": [],
            "variant_categories": [],
            "files": [],
            "images": [],
            "upsell_ids": [],
            "discount_codes": [
                {
                    "name": "LAUNCH20",
                    "amount_cents": 2000,
                    "offer_type": "percentage",
                    "max_purchase_count": None,
                    "expires_at": None
                }
            ],
            "affiliate_program": True,
            "affiliate_percentage": 7,
            "email_collection": True,
            "custom_domain": None
        },
        "marketing": {
            "short_description": product_info["description"][:100] + "..." if len(product_info["description"]) > 100 else product_info["description"],
            "detailed_description_markdown": generate_markdown_description(product_info),
            "features": [
                "Digital download",
                "Instant access",
                "Lifetime updates",
                "Money-back guarantee"
            ],
            "target_audience": [
                "Entrepreneurs",
                "Creators",
                "Freelancers",
                "Small business owners"
            ],
            "license": "Personal + commercial use allowed",
            "updates_policy": "Lifetime updates included",
            "refund_policy": "30-day money-back guarantee"
        }
    }
    
    # Add files with full paths
    base_path = product_info.get("base_path", "")
    files = product_info.get("files", [])
    
    for file_info in files:
        relative_path = file_info["path"]
        full_path = os.path.join(base_path, relative_path) if base_path else relative_path
        
        config["product"]["files"].append({
            "path": full_path,
            "display_name": file_info.get("display_name", os.path.basename(relative_path))
        })
    
    return config

def generate_markdown_description(product_info: dict) -> str:
    """Generate markdown description for product."""
    name = product_info["name"]
    description = product_info["description"]
    tags = product_info["tags"]
    
    markdown = f"# {name}\n\n"
    markdown += f"{description}\n\n"
    markdown += "## What You'll Get\n\n"
    markdown += "- Digital download in PDF format\n"
    markdown += "- Instant access after purchase\n"
    markdown += "- Lifetime updates included\n"
    markdown += "- 30-day money-back guarantee\n\n"
    
    if "files" in product_info:
        markdown += "## Included Files\n\n"
        for file_info in product_info["files"]:
            display_name = file_info.get("display_name", file_info["path"])
            markdown += f"- {display_name}\n"
    
    markdown += "\n## How It Works\n\n"
    markdown += "1. Purchase this digital product\n"
    markdown += "2. Download the files immediately\n"
    markdown += "3. Use the templates/resources for your business\n"
    markdown += "4. Get lifetime updates as we improve the product\n\n"
    
    markdown += "## Perfect For\n\n"
    markdown += "- Entrepreneurs and business owners\n"
    markdown += "- Content creators and freelancers\n"
    markdown += "- Anyone looking to streamline their workflow\n\n"
    
    markdown += f"## Tags\n\n"
    markdown += ", ".join([f"`{tag}`" for tag in tags]) + "\n\n"
    
    markdown += "---\n\n"
    markdown += "**Note**: This is a digital product. No physical items will be shipped.\n"
    
    return markdown

def save_config(config: dict, output_path: str):
    """Save configuration to JSON file."""
    os.makedirs(os.path.dirname(output_path), exist_ok=True)
    
    with open(output_path, 'w', encoding='utf-8') as f:
        json.dump(config, f, indent=2, ensure_ascii=False)
    
    print(f"✅ Generated: {output_path}")

def main():
    parser = argparse.ArgumentParser(description="Generate Gumroad configs from registry")
    parser.add_argument("--registry", "-r", default="product_registry.json",
                       help="Path to product registry JSON file")
    parser.add_argument("--output-dir", "-o", default="generated_configs",
                       help="Directory to save generated config files")
    parser.add_argument("--template", "-t", 
                       help="Path to template configuration (optional)")
    
    args = parser.parse_args()
    
    # Load registry
    registry = load_registry(args.registry)
    
    # Create output directory
    os.makedirs(args.output_dir, exist_ok=True)
    
    # Generate config for each product
    for product in registry.get("products", []):
        product_id = product.get("id", "unknown")
        config = generate_gumroad_config(product, args.template)
        
        output_path = os.path.join(args.output_dir, f"{product_id}_config.json")
        save_config(config, output_path)
    
    print(f"\n🎉 Generated {len(registry.get('products', []))} configuration files in '{args.output_dir}/'")
    print("\nNext steps:")
    print("1. Review the generated configuration files")
    print("2. Customize prices, descriptions, and files as needed")
    print("3. Use upload_product.py to upload to Gumroad")

if __name__ == "__main__":
    main()