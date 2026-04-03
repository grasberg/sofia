#!/usr/bin/env python3
import subprocess
import sys
import os

def markdown_to_html(markdown_content):
    """Convert markdown to HTML using pandoc"""
    try:
        result = subprocess.run(
            ['pandoc', '-f', 'markdown', '-t', 'html'],
            input=markdown_content.encode('utf-8'),
            capture_output=True,
            check=True
        )
        return result.stdout.decode('utf-8')
    except subprocess.CalledProcessError as e:
        print(f"Error converting markdown to HTML: {e}", file=sys.stderr)
        sys.exit(1)

def html_to_pdf(html_content, output_path):
    """Convert HTML to PDF using weasyprint"""
    try:
        result = subprocess.run(
            ['weasyprint', '-', output_path],
            input=html_content.encode('utf-8'),
            capture_output=True,
            check=True
        )
        return True
    except subprocess.CalledProcessError as e:
        print(f"Error converting HTML to PDF: {e}", file=sys.stderr)
        if e.stderr:
            print(f"stderr: {e.stderr.decode('utf-8')}", file=sys.stderr)
        sys.exit(1)

def main():
    if len(sys.argv) != 3:
        print(f"Usage: {sys.argv[0]} <input.md> <output.pdf>", file=sys.stderr)
        sys.exit(1)
    
    input_file = sys.argv[1]
    output_file = sys.argv[2]
    
    # Read markdown file
    try:
        with open(input_file, 'r', encoding='utf-8') as f:
            markdown_content = f.read()
    except FileNotFoundError:
        print(f"Error: Input file '{input_file}' not found", file=sys.stderr)
        sys.exit(1)
    
    # Convert markdown to HTML
    print("Converting markdown to HTML...")
    html_content = markdown_to_html(markdown_content)
    
    # Add basic styling
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
            padding: 20px;
        }}
        h1 {{
            color: #2c3e50;
            border-bottom: 2px solid #3498db;
            padding-bottom: 10px;
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
        }}
        .prompt-text {{
            font-style: italic;
            color: #2c3e50;
            background-color: #ecf0f1;
            padding: 10px;
            border-radius: 3px;
            margin: 10px 0;
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
    </style>
</head>
<body>
{html_content}
<footer>
    <p>Skapat med AI-skribent Prompts © {os.environ.get('USER', 'Användare')} • {os.path.basename(output_file)}</p>
</footer>
</body>
</html>"""
    
    # Convert HTML to PDF
    print("Converting HTML to PDF...")
    html_to_pdf(styled_html, output_file)
    
    print(f"PDF successfully created: {output_file}")
    print(f"File size: {os.path.getsize(output_file) if os.path.exists(output_file) else 'unknown'} bytes")

if __name__ == "__main__":
    main()