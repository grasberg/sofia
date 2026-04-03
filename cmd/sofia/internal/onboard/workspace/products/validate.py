#!/usr/bin/env python3
"""
Validate product configuration YAML files against schema.
"""

import yaml
import sys
import os
from typing import Dict, Any, List

def validate_core(core: Dict[str, Any]) -> List[str]:
    """Validate core product configuration."""
    errors = []
    
    required_fields = [
        'name', 'short_description', 'full_description', 
        'price', 'type', 'delivery_method', 'license',
        'files', 'category'
    ]
    
    for field in required_fields:
        if field not in core:
            errors.append(f"Missing required core field: {field}")
    
    if 'price' in core:
        price = core['price']
        if 'amount' not in price:
            errors.append("Price missing 'amount'")
        if 'currency' not in price:
            errors.append("Price missing 'currency'")
    
    if 'files' in core:
        files = core['files']
        if not isinstance(files, list):
            errors.append("Files must be a list")
        elif len(files) == 0:
            errors.append("At least one file must be specified")
        else:
            main_files = [f for f in files if f.get('is_main')]
            if len(main_files) == 0:
                errors.append("At least one file must be marked as main (is_main: true)")
    
    return errors

def validate_modules(modules: Dict[str, Any]) -> List[str]:
    """Validate optional modules."""
    errors = []
    
    if not isinstance(modules, dict):
        return ["Modules must be a dictionary"]
    
    # Check each module has enabled flag
    for module_name, module_data in modules.items():
        if not isinstance(module_data, dict):
            errors.append(f"Module '{module_name}' must be a dictionary")
            continue
        
        if 'enabled' not in module_data:
            errors.append(f"Module '{module_name}' missing 'enabled' flag")
        
        # If enabled, check for settings
        if module_data.get('enabled') and 'settings' not in module_data:
            errors.append(f"Enabled module '{module_name}' missing 'settings'")
    
    return errors

def validate_product(filepath: str) -> bool:
    """Validate a product YAML file."""
    try:
        with open(filepath, 'r', encoding='utf-8') as f:
            data = yaml.safe_load(f)
    except yaml.YAMLError as e:
        print(f"❌ YAML parsing error in {filepath}: {e}")
        return False
    except FileNotFoundError:
        print(f"❌ File not found: {filepath}")
        return False
    
    errors = []
    
    # Check top-level structure
    if 'core' not in data:
        errors.append("Missing 'core' section")
    else:
        errors.extend(validate_core(data['core']))
    
    if 'modules' in data:
        errors.extend(validate_modules(data['modules']))
    
    if 'metadata' not in data:
        errors.append("Missing 'metadata' section")
    
    if errors:
        print(f"❌ Validation failed for {filepath}:")
        for error in errors:
            print(f"   - {error}")
        return False
    
    print(f"✅ {filepath} is valid")
    return True

def main():
    """Main validation function."""
    if len(sys.argv) < 2:
        print("Usage: python validate.py <product.yaml> [<product2.yaml> ...]")
        sys.exit(1)
    
    all_valid = True
    for filepath in sys.argv[1:]:
        if not os.path.exists(filepath):
            print(f"❌ File does not exist: {filepath}")
            all_valid = False
            continue
        
        if not validate_product(filepath):
            all_valid = False
    
    if not all_valid:
        sys.exit(1)
    
    print("\n🎉 All product configurations are valid!")

if __name__ == '__main__':
    main()