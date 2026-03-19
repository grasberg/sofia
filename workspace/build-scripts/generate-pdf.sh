#!/bin/bash
# Build script för PDF-generation från Markdown-filer
# Använder md_to_pdf.py med pandoc och weasyprint

set -e

# Konfiguration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
PDF_OUTPUT_DIR="${PROJECT_ROOT}/dist/pdfs"
MD_SOURCE_DIR="${PROJECT_ROOT}/ai-prompts-product"

# Skapa utdatamapp
mkdir -p "$PDF_OUTPUT_DIR"

# Kontrollera beroenden
echo "Kontrollerar beroenden..."
if ! command -v pandoc &> /dev/null; then
    echo "Error: pandoc krävs men hittades inte. Installera med 'brew install pandoc' eller 'apt-get install pandoc'"
    exit 1
fi

if ! command -v weasyprint &> /dev/null; then
    echo "Error: weasyprint krävs men hittades inte. Installera med 'pip install weasyprint'"
    exit 1
fi

if ! command -v python3 &> /dev/null; then
    echo "Error: python3 krävs"
    exit 1
fi

# Generera PDF från alla markdown-filer i source-mappen
echo "Genererar PDF-filer från Markdown..."
for md_file in "$MD_SOURCE_DIR"/*.md; do
    if [ -f "$md_file" ]; then
        base_name=$(basename "$md_file" .md)
        output_pdf="${PDF_OUTPUT_DIR}/${base_name}.pdf"
        
        echo "  Konverterar: $(basename "$md_file") -> $(basename "$output_pdf")"
        
        # Kör python-skriptet
        python3 "${MD_SOURCE_DIR}/md_to_pdf.py" "$md_file" "$output_pdf"
        
        if [ $? -eq 0 ]; then
            echo "    ✓ Klart"
        else
            echo "    ✗ Misslyckades"
            exit 1
        fi
    fi
done

echo "PDF-generation slutförd. Filer sparade i: $PDF_OUTPUT_DIR"
ls -lh "$PDF_OUTPUT_DIR"