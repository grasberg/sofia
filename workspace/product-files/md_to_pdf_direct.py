#!/usr/bin/env python3
import os
import sys
from weasyprint import HTML
from markdown import markdown

def convert_md_to_pdf(input_path, output_path):
    # Read markdown file
    with open(input_path, 'r', encoding='utf-8') as f:
        md_content = f.read()
    
    # Convert markdown to HTML
    html_content = markdown(md_content, extensions=['extra', 'tables'])
    
    # Add styling and wrap in full HTML document
    styled_html = f"""<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <title>AI-skribent Prompts</title>
    <style>
        body {{
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            line-height: 1.6;
            color: #333;
            max-width: 800px;
            margin: 0 auto;
            padding: 40px;
        }}
        h1 {{
            color: #2c3e50;
            border-bottom: 2px solid #3498db;
            padding-bottom: 10px;
            margin-top: 0;
        }}
        h2 {{
            color: #34495e;
            margin-top: 30px;
            border-left: 4px solid #3498db;
            padding-left: 10px;
        }}
        h3 {{
            color: #7f8c8d;
        }}
        .prompt {{
            background-color: #f8f9fa;
            border: 1px solid #e9ecef;
            border-radius: 5px;
            padding: 15px;
            margin: 15px 0;
            page-break-inside: avoid;
        }}
        .prompt-text {{
            font-family: 'Courier New', monospace;
            font-size: 0.9em;
            color: #2c3e50;
            background-color: #ecf0f1;
            padding: 15px;
            border-radius: 3px;
            margin: 10px 0;
            white-space: pre-wrap;
            overflow-wrap: break-word;
        }}
        .usage {{
            background-color: #e8f4fc;
            border-left: 3px solid #3498db;
            padding: 10px;
            margin: 10px 0;
        }}
        .tip {{
            background-color: #fff8e1;
            border-left: 3px solid #ffc107;
            padding: 10px;
            margin: 10px 0;
        }}
        footer {{
            margin-top: 40px;
            padding-top: 20px;
            border-top: 1px solid #eee;
            color: #7f8c8d;
            font-size: 0.9em;
            text-align: center;
        }}
        @media print {{
            body {{
                padding: 20px;
            }}
        }}
    </style>
</head>
<body>
{html_content}
<footer>
    <p>AI Skribent Prompts © 2026 • Skapat med ❤️ för kreativa skribenter</p>
</footer>
</body>
</html>"""
    
    # Generate PDF
    HTML(string=styled_html).write_pdf(output_path)
    print(f"PDF skapad: {output_path}")
    print(f"Storlek: {os.path.getsize(output_path)} bytes")

if __name__ == "__main__":
    if len(sys.argv) != 3:
        print(f"Användning: {sys.argv[0]} <input.md> <output.pdf>", file=sys.stderr)
        sys.exit(1)
    
    input_file = sys.argv[1]
    output_file = sys.argv[2]
    
    if not os.path.exists(input_file):
        print(f"Fel: Indatafil '{input_file}' finns inte", file=sys.stderr)
        sys.exit(1)
    
    try:
        convert_md_to_pdf(input_file, output_file)
    except Exception as e:
        print(f"Fel vid PDF-generering: {e}", file=sys.stderr)
        sys.exit(1)