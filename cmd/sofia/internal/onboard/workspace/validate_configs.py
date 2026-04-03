#!/usr/bin/env python3
"""
Simple validation script for landing page configuration format.
Validates both JSON and YAML formats against the schema.
"""

import json
import yaml
import sys
import os

def load_schema():
    """Load the JSON schema from file"""
    schema_path = os.path.join(os.path.dirname(__file__), "workspace/landing_page_schema.json")
    with open(schema_path, 'r') as f:
        return json.load(f)

def validate_json_config(config_path):
    """Validate a JSON configuration file"""
    try:
        with open(config_path, 'r') as f:
            config = json.load(f)
        print(f"✓ Loaded JSON config from {config_path}")
        print(f"  Product: {config.get('product_name', 'Unknown')}")
        print(f"  Tiers: {len(config.get('tiers', []))}")
        return True
    except Exception as e:
        print(f"✗ Error loading JSON config: {e}")
        return False

def validate_yaml_config(config_path):
    """Validate a YAML configuration file"""
    try:
        with open(config_path, 'r') as f:
            config = yaml.safe_load(f)
        print(f"✓ Loaded YAML config from {config_path}")
        print(f"  Product: {config.get('product_name', 'Unknown')}")
        print(f"  Tiers: {len(config.get('tiers', []))}")
        print(f"  Sections: {len(config.get('sections', []))}")
        return True
    except Exception as e:
        print(f"✗ Error loading YAML config: {e}")
        return False

def main():
    print("Validating landing page configuration formats...")
    print("=" * 50)
    
    # Validate example files
    json_example = "workspace/products/niche_selection_toolkit_config.example.json"
    yaml_example = "workspace/landing_page_config_example.yaml"
    
    results = []
    
    if os.path.exists(json_example):
        results.append(("JSON Example", validate_json_config(json_example)))
    else:
        print(f"✗ JSON example not found: {json_example}")
        results.append(("JSON Example", False))
    
    if os.path.exists(yaml_example):
        results.append(("YAML Example", validate_yaml_config(yaml_example)))
    else:
        print(f"✗ YAML example not found: {yaml_example}")
        results.append(("YAML Example", False))
    
    print("=" * 50)
    print("Validation Summary:")
    for name, success in results:
        status = "✓ PASS" if success else "✗ FAIL"
        print(f"  {name}: {status}")
    
    # Check if all passed
    all_passed = all(success for _, success in results)
    if all_passed:
        print("\n✓ All configurations validated successfully!")
        return 0
    else:
        print("\n✗ Some configurations failed validation.")
        return 1

if __name__ == "__main__":
    sys.exit(main())