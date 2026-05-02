// ABOUTME: Implements the symlink create/remove algorithm used by both sync binaries.
// ABOUTME: For selected items it creates symlinks; for deselected items it removes only its own symlinks.

package sync

import (
	"fmt"
	"os"
	"path/filepath"
)

// Result holds the outcome of a SyncItems or BuildTemplate call.
type Result struct {
	// Linked is the number of symlinks successfully created or updated.
	Linked int
	// Removed is the number of symlinks removed because the item was deselected.
	Removed int
	// Built is the number of template files successfully built.
	Built int
}

// SyncItems applies the symlink algorithm for all items in allItems.
//
// For each item in allItems:
//   - If the item is in selected: create parent dirs and run ln -sf (os.Symlink
//     with prior removal of any existing entry at dest).
//   - If the item is NOT in selected: if dest is a symlink whose target equals
//     src (via os.Readlink, not filepath.EvalSymlinks), remove dest.
//
// The src path is built as: srcBase/item
// The dest path is built as: destBase/item
//
// Args:
//
//	allItems  ([]string):     All relative item paths discovered in the source tree.
//	selected  (map[string]bool): Set of relative paths the user chose to sync.
//	srcBase   (string):       Absolute path to the source directory.
//	destBase  (string):       Absolute path to the destination directory.
//
// Returns:
//
//	result (Result): Count of linked and removed items.
//	err    (error):  First error encountered, or nil on success.
func SyncItems(allItems []string, selected map[string]bool, srcBase, destBase string) (Result, error) {
	var res Result

	for _, item := range allItems {
		src := filepath.Join(srcBase, item)
		dest := filepath.Join(destBase, item)

		if selected[item] {
			// Ensure parent directory exists.
			if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
				return res, fmt.Errorf("mkdir -p %s: %w", filepath.Dir(dest), err)
			}

			// Remove any existing entry at dest (file, symlink, or directory)
			// before creating the new symlink, mirroring `ln -sf`.
			if err := os.Remove(dest); err != nil && !os.IsNotExist(err) {
				return res, fmt.Errorf("remove existing %s: %w", dest, err)
			}

			if err := os.Symlink(src, dest); err != nil {
				return res, fmt.Errorf("symlink %s -> %s: %w", dest, src, err)
			}

			displayPath := dest
			if abs, err := filepath.Abs(dest); err == nil {
				displayPath = abs
			}
			fmt.Printf("  linked: %s\n", displayPath)
			res.Linked++
		} else {
			// Only remove dest if it is a symlink pointing exactly to src.
			target, err := os.Readlink(dest)
			if err != nil {
				// dest is not a symlink or does not exist — skip silently.
				continue
			}
			if target == src {
				if err := os.Remove(dest); err != nil && !os.IsNotExist(err) {
					return res, fmt.Errorf("remove %s: %w", dest, err)
				}
				displayPath := dest
				if abs, err := filepath.Abs(dest); err == nil {
					displayPath = abs
				}
				fmt.Printf("  removed: %s\n", displayPath)
				res.Removed++
			}
		}
	}

	return res, nil
}
