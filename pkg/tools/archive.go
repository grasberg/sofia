package tools

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ArchiveTool provides compression and archiving operations using Go stdlib.
type ArchiveTool struct {
	workspace string
	restrict  bool
}

func NewArchiveTool(workspace string, restrict bool) *ArchiveTool {
	return &ArchiveTool{workspace: workspace, restrict: restrict}
}

func (t *ArchiveTool) Name() string { return "archive" }
func (t *ArchiveTool) Description() string {
	return "Create and extract archives (zip, tar, tar.gz). Actions: create (compress files), extract (decompress archive), list (show archive contents)."
}

func (t *ArchiveTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"description": "Action to perform",
				"enum":        []string{"create", "extract", "list"},
			},
			"format": map[string]any{
				"type":        "string",
				"description": "Archive format",
				"enum":        []string{"zip", "tar", "tar.gz"},
			},
			"archive_path": map[string]any{
				"type":        "string",
				"description": "Path to the archive file",
			},
			"files": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "string",
				},
				"description": "Files/directories to include (for create action)",
			},
			"dest": map[string]any{
				"type":        "string",
				"description": "Destination directory (for extract action)",
			},
		},
		"required": []string{"action", "archive_path"},
	}
}

func (t *ArchiveTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	action, _ := args["action"].(string)
	archivePath, _ := args["archive_path"].(string)

	if archivePath == "" {
		return ErrorResult("archive_path is required")
	}

	if t.restrict {
		resolved, err := validatePath(archivePath, t.workspace, true)
		if err != nil {
			return ErrorResult(err.Error())
		}
		archivePath = resolved
	} else if !filepath.IsAbs(archivePath) {
		archivePath = filepath.Join(t.workspace, archivePath)
	}

	format := ""
	if f, ok := args["format"].(string); ok {
		format = f
	}
	if format == "" {
		format = detectArchiveFormat(archivePath)
	}

	switch action {
	case "create":
		return t.createArchive(ctx, archivePath, format, args)
	case "extract":
		return t.extractArchive(ctx, archivePath, format, args)
	case "list":
		return t.listArchive(archivePath, format)
	default:
		return ErrorResult(fmt.Sprintf("unknown action: %s", action))
	}
}

func detectArchiveFormat(path string) string {
	lower := strings.ToLower(path)
	switch {
	case strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tgz"):
		return "tar.gz"
	case strings.HasSuffix(lower, ".tar"):
		return "tar"
	case strings.HasSuffix(lower, ".zip"):
		return "zip"
	default:
		return "zip"
	}
}

func (t *ArchiveTool) createArchive(_ context.Context, archivePath, format string, args map[string]any) *ToolResult {
	var files []string
	if raw, ok := args["files"]; ok {
		parsed, err := parseStringArgs(raw)
		if err != nil {
			return ErrorResult("files must be an array of strings")
		}
		files = parsed
	}
	if len(files) == 0 {
		return ErrorResult("files is required for create action")
	}

	// Resolve file paths
	for i, f := range files {
		if !filepath.IsAbs(f) {
			files[i] = filepath.Join(t.workspace, f)
		}
		if t.restrict {
			resolved, err := validatePath(files[i], t.workspace, true)
			if err != nil {
				return ErrorResult(err.Error())
			}
			files[i] = resolved
		}
	}

	switch format {
	case "zip":
		return t.createZip(archivePath, files)
	case "tar", "tar.gz":
		return t.createTar(archivePath, files, format == "tar.gz")
	default:
		return ErrorResult(fmt.Sprintf("unsupported format: %s", format))
	}
}

func (t *ArchiveTool) createZip(archivePath string, files []string) *ToolResult {
	if err := os.MkdirAll(filepath.Dir(archivePath), 0o755); err != nil {
		return ErrorResult(fmt.Sprintf("failed to create directory: %v", err))
	}

	outFile, err := os.Create(archivePath)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to create archive: %v", err))
	}
	defer outFile.Close()

	w := zip.NewWriter(outFile)
	defer w.Close()

	count := 0
	for _, path := range files {
		err := filepath.WalkDir(path, func(fpath string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			relPath, _ := filepath.Rel(t.workspace, fpath)
			writer, err := w.Create(relPath)
			if err != nil {
				return err
			}
			f, err := os.Open(fpath)
			if err != nil {
				return err
			}
			defer f.Close()
			_, err = io.Copy(writer, f)
			count++
			return err
		})
		if err != nil {
			return ErrorResult(fmt.Sprintf("error adding files: %v", err))
		}
	}

	return SilentResult(fmt.Sprintf("Created %s with %d files", archivePath, count))
}

func (t *ArchiveTool) createTar(archivePath string, files []string, gzipped bool) *ToolResult {
	if err := os.MkdirAll(filepath.Dir(archivePath), 0o755); err != nil {
		return ErrorResult(fmt.Sprintf("failed to create directory: %v", err))
	}

	outFile, err := os.Create(archivePath)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to create archive: %v", err))
	}
	defer outFile.Close()

	var tw *tar.Writer
	if gzipped {
		gw := gzip.NewWriter(outFile)
		defer gw.Close()
		tw = tar.NewWriter(gw)
	} else {
		tw = tar.NewWriter(outFile)
	}
	defer tw.Close()

	count := 0
	for _, path := range files {
		err := filepath.WalkDir(path, func(fpath string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			header, err := tar.FileInfoHeader(info, "")
			if err != nil {
				return err
			}
			relPath, _ := filepath.Rel(t.workspace, fpath)
			header.Name = relPath
			if err := tw.WriteHeader(header); err != nil {
				return err
			}
			f, err := os.Open(fpath)
			if err != nil {
				return err
			}
			defer f.Close()
			_, err = io.Copy(tw, f)
			count++
			return err
		})
		if err != nil {
			return ErrorResult(fmt.Sprintf("error adding files: %v", err))
		}
	}

	return SilentResult(fmt.Sprintf("Created %s with %d files", archivePath, count))
}

func (t *ArchiveTool) extractArchive(_ context.Context, archivePath, format string, args map[string]any) *ToolResult {
	dest := t.workspace
	if d, ok := args["dest"].(string); ok && d != "" {
		dest = d
		if !filepath.IsAbs(dest) {
			dest = filepath.Join(t.workspace, dest)
		}
	}

	if t.restrict {
		resolved, err := validatePath(dest, t.workspace, true)
		if err != nil {
			return ErrorResult(err.Error())
		}
		dest = resolved
	}

	switch format {
	case "zip":
		return t.extractZip(archivePath, dest)
	case "tar", "tar.gz":
		return t.extractTar(archivePath, dest, format == "tar.gz")
	default:
		return ErrorResult(fmt.Sprintf("unsupported format: %s", format))
	}
}

func (t *ArchiveTool) extractZip(archivePath, dest string) *ToolResult {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to open zip: %v", err))
	}
	defer r.Close()

	count := 0
	for _, f := range r.File {
		fpath := filepath.Join(dest, f.Name)
		// Prevent zip slip
		if !strings.HasPrefix(filepath.Clean(fpath), filepath.Clean(dest)+string(os.PathSeparator)) {
			continue
		}

		if f.FileInfo().IsDir() {
			os.MkdirAll(fpath, 0o755)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(fpath), 0o755); err != nil {
			continue
		}

		rc, err := f.Open()
		if err != nil {
			continue
		}

		outFile, err := os.Create(fpath)
		if err != nil {
			rc.Close()
			continue
		}
		io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()
		count++
	}

	return SilentResult(fmt.Sprintf("Extracted %d files to %s", count, dest))
}

func (t *ArchiveTool) extractTar(archivePath, dest string, gzipped bool) *ToolResult {
	f, err := os.Open(archivePath)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to open archive: %v", err))
	}
	defer f.Close()

	var reader io.Reader = f
	if gzipped {
		gr, err := gzip.NewReader(f)
		if err != nil {
			return ErrorResult(fmt.Sprintf("failed to open gzip: %v", err))
		}
		defer gr.Close()
		reader = gr
	}

	tr := tar.NewReader(reader)
	count := 0
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return ErrorResult(fmt.Sprintf("tar read error: %v", err))
		}

		fpath := filepath.Join(dest, header.Name)
		// Prevent tar slip
		if !strings.HasPrefix(filepath.Clean(fpath), filepath.Clean(dest)+string(os.PathSeparator)) {
			continue
		}

		switch header.Typeflag {
		case tar.TypeDir:
			os.MkdirAll(fpath, 0o755)
		case tar.TypeReg:
			os.MkdirAll(filepath.Dir(fpath), 0o755)
			outFile, err := os.Create(fpath)
			if err != nil {
				continue
			}
			io.Copy(outFile, tr)
			outFile.Close()
			count++
		}
	}

	return SilentResult(fmt.Sprintf("Extracted %d files to %s", count, dest))
}

func (t *ArchiveTool) listArchive(archivePath, format string) *ToolResult {
	switch format {
	case "zip":
		return t.listZip(archivePath)
	case "tar", "tar.gz":
		return t.listTar(archivePath, format == "tar.gz")
	default:
		return ErrorResult(fmt.Sprintf("unsupported format: %s", format))
	}
}

func (t *ArchiveTool) listZip(archivePath string) *ToolResult {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to open zip: %v", err))
	}
	defer r.Close()

	var sb strings.Builder
	for _, f := range r.File {
		sb.WriteString(fmt.Sprintf("%10d  %s  %s\n",
			f.UncompressedSize64, f.Modified.Format("2006-01-02 15:04"), f.Name))
	}
	sb.WriteString(fmt.Sprintf("\n(%d entries)", len(r.File)))
	return NewToolResult(sb.String())
}

func (t *ArchiveTool) listTar(archivePath string, gzipped bool) *ToolResult {
	f, err := os.Open(archivePath)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to open archive: %v", err))
	}
	defer f.Close()

	var reader io.Reader = f
	if gzipped {
		gr, err := gzip.NewReader(f)
		if err != nil {
			return ErrorResult(fmt.Sprintf("failed to open gzip: %v", err))
		}
		defer gr.Close()
		reader = gr
	}

	tr := tar.NewReader(reader)
	var sb strings.Builder
	count := 0
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
		sb.WriteString(fmt.Sprintf("%10d  %s  %s\n",
			header.Size, header.ModTime.Format("2006-01-02 15:04"), header.Name))
		count++
	}
	sb.WriteString(fmt.Sprintf("\n(%d entries)", count))
	return NewToolResult(sb.String())
}
