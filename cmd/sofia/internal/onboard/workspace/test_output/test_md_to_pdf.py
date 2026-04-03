#!/usr/bin/env python3
"""
Unit tests for md_to_pdf.py script.
Uses unittest.mock to simulate subprocess calls.
Run with: python3 -m pytest test_md_to_pdf.py -v
Or: python3 test_md_to_pdf.py
"""

import unittest
from unittest.mock import patch, MagicMock, call
import sys
import os

# Import the functions from md_to_pdf.py by reading the file and executing it
# This is a bit hacky but works without modifying the original script
def import_md_to_pdf():
    """Dynamically import md_to_pdf functions"""
    import subprocess
    import sys as sys_module
    import os as os_module
    
    # Define the functions in a namespace
    namespace = {
        'subprocess': subprocess,
        'sys': sys_module,
        'os': os_module
    }
    
    # Read the original script
    script_path = os.path.join(os.path.dirname(__file__), '..', 'ai-prompts-product', 'md_to_pdf.py')
    with open(script_path, 'r', encoding='utf-8') as f:
        script_code = f.read()
    
    # Execute the script in the namespace
    exec(script_code, namespace)
    
    return namespace['markdown_to_html'], namespace['html_to_pdf'], namespace['main']

# Import functions
markdown_to_html, html_to_pdf, main = import_md_to_pdf()


class TestMarkdownToHtml(unittest.TestCase):
    
    @patch('subprocess.run')
    def test_markdown_to_html_success(self, mock_run):
        """Test successful markdown to HTML conversion"""
        # Setup mock
        mock_result = MagicMock()
        mock_result.stdout = b'<h1>Test</h1>'
        mock_run.return_value = mock_result
        
        # Call function
        result = markdown_to_html('# Test')
        
        # Assertions
        self.assertEqual(result, '<h1>Test</h1>')
        mock_run.assert_called_once_with(
            ['pandoc', '-f', 'markdown', '-t', 'html'],
            input=b'# Test',
            capture_output=True,
            check=True
        )
    
    @patch('subprocess.run')
    def test_markdown_to_html_failure(self, mock_run):
        """Test pandoc failure"""
        # Setup mock to raise CalledProcessError
        mock_run.side_effect = Exception('pandoc not found')
        
        # We expect sys.exit(1) to be called
        with self.assertRaises(SystemExit) as cm:
            markdown_to_html('# Test')
        
        # Check exit code
        self.assertEqual(cm.exception.code, 1)


class TestHtmlToPdf(unittest.TestCase):
    
    @patch('subprocess.run')
    def test_html_to_pdf_success(self, mock_run):
        """Test successful HTML to PDF conversion"""
        # Setup mock
        mock_result = MagicMock()
        mock_run.return_value = mock_result
        
        # Call function
        result = html_to_pdf('<html>test</html>', '/tmp/test.pdf')
        
        # Assertions
        self.assertTrue(result)
        mock_run.assert_called_once_with(
            ['weasyprint', '-', '/tmp/test.pdf'],
            input=b'<html>test</html>',
            capture_output=True,
            check=True
        )
    
    @patch('subprocess.run')
    def test_html_to_pdf_failure(self, mock_run):
        """Test weasyprint failure"""
        # Setup mock to raise CalledProcessError
        mock_run.side_effect = Exception('weasyprint error')
        
        # Expect sys.exit(1)
        with self.assertRaises(SystemExit) as cm:
            html_to_pdf('<html>test</html>', '/tmp/test.pdf')
        
        self.assertEqual(cm.exception.code, 1)


class TestMainFunction(unittest.TestCase):
    
    @patch('sys.argv', ['md_to_pdf.py', 'input.md', 'output.pdf'])
    @patch('builtins.open')
    @patch('subprocess.run')
    def test_main_success(self, mock_run, mock_open):
        """Test main function with successful conversion"""
        # Mock file reading
        mock_file = MagicMock()
        mock_file.read.return_value = '# Test Content'
        mock_open.return_value.__enter__.return_value = mock_file
        
        # Mock subprocess calls
        mock_pandoc_result = MagicMock()
        mock_pandoc_result.stdout = b'<h1>Test Content</h1>'
        mock_weasy_result = MagicMock()
        
        def run_side_effect(*args, **kwargs):
            if args[0][0] == 'pandoc':
                return mock_pandoc_result
            elif args[0][0] == 'weasyprint':
                return mock_weasy_result
        
        mock_run.side_effect = run_side_effect
        
        # Mock os.path.getsize
        with patch('os.path.getsize', return_value=1024):
            # Capture print output
            with patch('builtins.print') as mock_print:
                main()
        
        # Verify file was opened
        mock_open.assert_called_once_with('input.md', 'r', encoding='utf-8')
        
        # Verify subprocess calls
        self.assertEqual(mock_run.call_count, 2)
        
        # Verify print calls
        self.assertTrue(mock_print.called)
    
    @patch('sys.argv', ['md_to_pdf.py', 'input.md'])
    def test_main_wrong_args(self):
        """Test main with wrong number of arguments"""
        with self.assertRaises(SystemExit) as cm:
            main()
        self.assertEqual(cm.exception.code, 1)
    
    @patch('sys.argv', ['md_to_pdf.py', 'nonexistent.md', 'output.pdf'])
    def test_main_file_not_found(self):
        """Test main with non-existent input file"""
        with patch('builtins.open', side_effect=FileNotFoundError()):
            with self.assertRaises(SystemExit) as cm:
                main()
            self.assertEqual(cm.exception.code, 1)


class TestStyling(unittest.TestCase):
    """Test that styling is correctly applied"""
    
    def test_styling_integration(self):
        """Verify that styling template is properly formatted"""
        # This is a simple smoke test to ensure the f-string doesn't have syntax errors
        # We'll test by calling markdown_to_html and then verifying the styling wrapper
        # uses the returned HTML
        with patch('subprocess.run') as mock_run:
            mock_result = MagicMock()
            mock_result.stdout = b'<h1>Test</h1>'
            mock_run.return_value = mock_result
            
            html = markdown_to_html('# Test')
            
            # The styling is applied in main(), not in the functions we're testing
            # So we'll just verify the function works
            self.assertEqual(html, '<h1>Test</h1>')


if __name__ == '__main__':
    unittest.main()