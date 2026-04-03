package health

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// DatabaseChecker is satisfied by any type that has a Ping() method
// (e.g. memory.MemoryDB).
type DatabaseChecker interface {
	Ping() error
}

// DatabaseCheck returns a health check function that pings the database.
func DatabaseCheck(db DatabaseChecker) func() (bool, string) {
	return func() (bool, string) {
		if db == nil {
			return false, "database not initialized"
		}
		if err := db.Ping(); err != nil {
			return false, fmt.Sprintf("database ping failed: %v", err)
		}
		return true, "ok"
	}
}

// DiskSpaceCheck returns a health check function that verifies the data
// directory has at least minFreeBytes of free space. Pass 0 for minFreeBytes
// to use the default of 50 MB.
func DiskSpaceCheck(dataDir string, minFreeBytes uint64) func() (bool, string) {
	if minFreeBytes == 0 {
		minFreeBytes = 50 * 1024 * 1024 // 50 MB default
	}
	return func() (bool, string) {
		// Resolve to an existing directory — walk up if the leaf does not exist yet.
		dir := dataDir
		for dir != "" && dir != "/" {
			if info, err := os.Stat(dir); err == nil && info.IsDir() {
				break
			}
			dir = filepath.Dir(dir)
		}
		if dir == "" {
			dir = "/"
		}

		var stat syscall.Statfs_t
		if err := syscall.Statfs(dir, &stat); err != nil {
			return false, fmt.Sprintf("statfs failed: %v", err)
		}

		freeBytes := stat.Bavail * uint64(stat.Bsize)
		if freeBytes < minFreeBytes {
			return false, fmt.Sprintf("low disk space: %d MB free (minimum %d MB)",
				freeBytes/(1024*1024), minFreeBytes/(1024*1024))
		}

		return true, fmt.Sprintf("ok (%d MB free)", freeBytes/(1024*1024))
	}
}
