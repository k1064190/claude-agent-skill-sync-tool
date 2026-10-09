// ABOUTME: Implements the symlink create/remove algorithm used by both sync binaries.
// ABOUTME: For selected items it creates symlinks; for deselected items it removes only its own symlinks.

package sync

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Result holds the outcome of a SyncItems or BuildTemplate call.
type Result struct {
	// Linked is the number of symlinks successfully created or updated.
	Linked int
	// Removed is the number of symlinks removed because the item was deselected.
	Removed int
	// Built is the number of template files successfully built.
	Built int
	// Merged is the number of settings files updated from a fragment.
	Merged int
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
	return SyncItemsAs(allItems, selected, srcBase, destBase, IdentityDestName)
}

// IdentityDestName keeps an item's source-relative path as its destination path.
func IdentityDestName(item string) string { return item }

// SyncItemsAs is SyncItems with a destination name mapper: the dest path is
// destBase/destName(item) instead of destBase/item. Platforms that only
// discover one directory level (Claude skills) map nested items to their
// basename.
func SyncItemsAs(allItems []string, selected map[string]bool, srcBase, destBase string, destName func(string) string) (Result, error) {
	var res Result

	for _, item := range allItems {
		src := filepath.Join(srcBase, item)
		dest := filepath.Join(destBase, destName(item))

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

// CheckDestNameCollisions reports an error when two items map to the same
// destination name, which would make one link silently overwrite the other.
//
// Args:
//
//	allItems ([]string):            Relative item paths discovered in the source tree.
//	destName (func(string) string): Destination name mapper.
//
// Returns:
//
//	err (error): Collision description, or nil when every name is unique.
func CheckDestNameCollisions(allItems []string, destName func(string) string) error {
	seen := make(map[string]string, len(allItems))
	for _, item := range allItems {
		name := destName(item)
		if prev, ok := seen[name]; ok {
			return fmt.Errorf("items %q and %q both map to destination %q", prev, item, name)
		}
		seen[name] = item
	}
	return nil
}

// MigrateNestedLinks removes legacy links created at destBase/item for items
// whose destination name now differs (destName(item) != item), and prunes the
// directories left empty under destBase. Only symlinks pointing exactly at
// srcBase/item are touched.
//
// Args:
//
//	allItems ([]string):            Relative item paths discovered in the source tree.
//	srcBase  (string):              Absolute path to the source directory.
//	destBase (string):              Absolute path to the destination directory.
//	destName (func(string) string): Destination name mapper.
//
// Returns:
//
//	migrated (map[string]bool): Items whose legacy link was removed, so callers can relink them.
//	err      (error):           First removal error, or nil.
func MigrateNestedLinks(allItems []string, srcBase, destBase string, destName func(string) string) (map[string]bool, error) {
	migrated := make(map[string]bool)
	for _, item := range allItems {
		if destName(item) == item {
			continue
		}
		legacy := filepath.Join(destBase, item)
		target, err := os.Readlink(legacy)
		if err != nil || target != filepath.Join(srcBase, item) {
			continue
		}
		if err := os.Remove(legacy); err != nil {
			return migrated, fmt.Errorf("remove legacy %s: %w", legacy, err)
		}
		fmt.Printf("  migrated: %s\n", legacy)
		migrated[item] = true
		// Prune now-empty parents up to (but excluding) destBase. os.Remove
		// fails on a non-empty dir, which stops the walk.
		for dir := filepath.Dir(legacy); dir != destBase && strings.HasPrefix(dir, destBase+string(os.PathSeparator)); dir = filepath.Dir(dir) {
			if os.Remove(dir) != nil {
				break
			}
		}
	}
	return migrated, nil
}
