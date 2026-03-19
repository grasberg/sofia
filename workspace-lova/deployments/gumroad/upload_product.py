#!/usr/bin/env python3
"""
Gumroad Product Upload Script

This script uploads a digital product to Gumroad using the Gumroad API.
It reads a product configuration file (JSON) and uploads all associated files.

Requirements:
- Python 3.6+
- requests library (pip install requests)
- Gumroad access token with appropriate permissions

Usage:
    python3 upload_product.py --config product_config.json --token YOUR_ACCESS_TOKEN

Environment variables:
    GUMROAD_ACCESS_TOKEN can be used instead of --token flag
"""

import json
import os
import sys
import argparse
import requests
from pathlib import Path
from typing import Dict, Any, List, Optional

# Gumroad API endpoints
GUMROAD_API_BASE = "https://api.gumroad.com/v2"
PRODUCTS_ENDPOINT = f"{GUMROAD_API_BASE}/products"

class GumroadUploader:
    def __init__(self, access_token: str):
        self.access_token = access_token
        self.session = requests.Session()
        self.session.headers.update({
            "Authorization": f"Bearer {access_token}",
            "Content-Type": "application/json",
            "Accept": "application/json"
        })
    
    def create_product(self, product_data: Dict[str, Any]) -> Dict[str, Any]:
        """
        Create a new product on Gumroad.
        
        Args:
            product_data: Product configuration following Gumroad API schema
            
        Returns:
            API response JSON
        """
        # Prepare the payload for Gumroad API
        payload = self._prepare_product_payload(product_data)
        
        print(f"Creating product: {product_data.get('name', 'Unnamed product')}")
        response = self.session.post(PRODUCTS_ENDPOINT, json=payload)
        
        if response.status_code == 200:
            result = response.json()
            if result.get("success"):
                print(f"✅ Product created successfully! ID: {result.get('product', {}).get('id')}")
                return result
            else:
                error_msg = result.get("message", "Unknown error")
                print(f"❌ Failed to create product: {error_msg}")
                raise Exception(f"Gumroad API error: {error_msg}")
        else:
            print(f"❌ HTTP {response.status_code}: {response.text}")
            response.raise_for_status()
    
    def _prepare_product_payload(self, product_data: Dict[str, Any]) -> Dict[str, Any]:
        """
        Transform our configuration format to Gumroad API expected format.
        """
        # Extract product and marketing sections
        product_info = product_data.get("product", {})
        marketing_info = product_data.get("marketing", {})
        
        # Map our fields to Gumroad API fields
        payload = {
            "name": product_info.get("name"),
            "description": marketing_info.get("detailed_description_markdown", 
                                            product_info.get("description", "")),
            "price_cents": product_info.get("price_cents"),
            "currency": product_info.get("currency", "USD"),
            "tags": ", ".join(product_info.get("tags", [])),
            "category": product_info.get("category"),
            "custom_permalink": product_info.get("custom_permalink"),
            "max_purchase_count": product_info.get("quantity_available") if product_info.get("quantity_available") != "unlimited" else None,
            "custom_receipt": marketing_info.get("custom_receipt"),
            "custom_summary": marketing_info.get("short_description"),
            "custom_fields": product_info.get("custom_fields", []),
            "variant_categories": product_info.get("variant_categories", []),
            "is_adult": product_info.get("is_adult", False),
            "is_tiered_membership": product_info.get("is_tiered_membership", False),
        }
        
        # Remove None values
        payload = {k: v for k, v in payload.items() if v is not None}
        
        return payload
    
    def upload_file(self, product_id: str, file_path: str, display_name: str = None) -> Dict[str, Any]:
        """
        Upload a file to an existing product.
        
        Note: Gumroad API requires files to be uploaded via multipart/form-data
        after product creation. This is a placeholder for the actual implementation.
        """
        print(f"⚠️ File upload not implemented yet for {file_path}")
        print(f"   Manually upload files to product {product_id} in Gumroad dashboard")
        return {"status": "not_implemented", "file": file_path}
    
    def create_discount_code(self, product_id: str, discount_data: Dict[str, Any]) -> Dict[str, Any]:
        """
        Create a discount code for a product.
        """
        endpoint = f"{GUMROAD_API_BASE}/products/{product_id}/offer_codes"
        payload = {
            "name": discount_data.get("name"),
            "amount_off_cents": discount_data.get("amount_cents") if discount_data.get("offer_type") == "amount" else None,
            "percent_off": discount_data.get("amount_cents") // 100 if discount_data.get("offer_type") == "percentage" else None,
            "max_purchase_count": discount_data.get("max_purchase_count"),
            "expires_at": discount_data.get("expires_at"),
        }
        payload = {k: v for k, v in payload.items() if v is not None}
        
        print(f"Creating discount code: {discount_data.get('name')}")
        response = self.session.post(endpoint, json=payload)
        
        if response.status_code == 200:
            result = response.json()
            if result.get("success"):
                print(f"✅ Discount code created successfully!")
                return result
            else:
                error_msg = result.get("message", "Unknown error")
                print(f"❌ Failed to create discount code: {error_msg}")
                return result
        else:
            print(f"❌ HTTP {response.status_code}: {response.text}")
            return {"success": False, "error": response.text}

def load_config(config_path: str) -> Dict[str, Any]:
    """Load and validate configuration file."""
    try:
        with open(config_path, 'r', encoding='utf-8') as f:
            config = json.load(f)
        
        # Basic validation
        if "product" not in config:
            raise ValueError("Configuration must contain 'product' section")
        
        return config
    except json.JSONDecodeError as e:
        print(f"❌ Invalid JSON in config file: {e}")
        sys.exit(1)
    except FileNotFoundError:
        print(f"❌ Config file not found: {config_path}")
        sys.exit(1)

def main():
    parser = argparse.ArgumentParser(description="Upload product to Gumroad")
    parser.add_argument("--config", "-c", required=True, help="Path to product configuration JSON file")
    parser.add_argument("--token", "-t", help="Gumroad access token (or set GUMROAD_ACCESS_TOKEN env var)")
    parser.add_argument("--dry-run", action="store_true", help="Validate config without uploading")
    
    args = parser.parse_args()
    
    # Get access token
    access_token = args.token or os.environ.get("GUMROAD_ACCESS_TOKEN")
    if not access_token:
        print("❌ No access token provided. Use --token or set GUMROAD_ACCESS_TOKEN environment variable.")
        sys.exit(1)
    
    # Load configuration
    config = load_config(args.config)
    
    if args.dry_run:
        print("✅ Configuration is valid (dry run)")
        print(f"   Product: {config.get('product', {}).get('name', 'Unnamed')}")
        return
    
    # Initialize uploader
    uploader = GumroadUploader(access_token)
    
    try:
        # Step 1: Create product
        result = uploader.create_product(config)
        product_id = result.get("product", {}).get("id")
        
        if not product_id:
            print("❌ No product ID returned from API")
            sys.exit(1)
        
        # Step 2: Upload files (if any)
        files = config.get("product", {}).get("files", [])
        for file_info in files:
            file_path = file_info.get("path")
            display_name = file_info.get("display_name")
            if os.path.exists(file_path):
                uploader.upload_file(product_id, file_path, display_name)
            else:
                print(f"⚠️ File not found, skipping: {file_path}")
        
        # Step 3: Create discount codes (if any)
        discount_codes = config.get("product", {}).get("discount_codes", [])
        for discount in discount_codes:
            uploader.create_discount_code(product_id, discount)
        
        print("\n" + "="*50)
        print(f"✅ Product setup completed!")
        print(f"   Product ID: {product_id}")
        print(f"   Next steps:")
        print(f"   1. Manually upload files via Gumroad dashboard")
        print(f"   2. Configure any additional settings")
        print(f"   3. Preview and publish the product")
        print("="*50)
        
    except Exception as e:
        print(f"❌ Error during upload: {e}")
        sys.exit(1)

if __name__ == "__main__":
    main()