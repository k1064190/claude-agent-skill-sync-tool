// ABOUTME: Converts a sorted list of relative paths into a flat display node list for the TUI.
// ABOUTME: Inserts directory header nodes lazily and mirrors the bash _tui_tree_build function.

package tree

import (
	"path"
	"strings"
)

// NodeType distinguishes a directory header node from a selectable leaf node.
type NodeType int

const (
	// NodeTypeDir represents a directory grouping node (non-selectable, shows [x]/[ ]/[~]).
	NodeTypeDir NodeType = iota
	// NodeTypeLeaf represents a selectable leaf item (agent .md file or skill directory).
	NodeTypeLeaf
)

// Node holds the display state for a single row in the tree TUI.
// Dir nodes use OrigPath == "" and Selected is not meaningful for selection output;
// their checkbox state is derived from their descendants at render time.
type Node struct {
	// Label is the display string shown in the TUI row (basename or "dirname/").
	Label string
	// Type distinguishes dir header nodes from selectable leaf nodes.
	Type NodeType
	// Indent is the nesting depth: 0 for root-level, 1 for one level deep, 2 for two levels.
	Indent int
	// OrigPath is the full relative path of the original item (non-empty only for leaves).
	OrigPath string
	// Selected holds the current selection state for leaf nodes (true = selected).
	Selected bool
}

// BuildNodes takes a sorted slice of relative paths and produces a flat list of
// Node values suitable for rendering in the tree TUI. Directory header nodes are
// inserted lazily on the first encounter of each unique directory component.
// Supports paths up to three components deep (leaf, dir/leaf, dir/sub/leaf).
//
// Args:
//
//	items ([]string): Sorted relative paths, e.g. ["business/pm.md", "dev/go.md"].
//
// Returns:
//
//	nodes ([]Node): Flat ordered list of display nodes (dirs followed by their children).
//	leafCount (int): Number of leaf nodes in the returned slice.
func BuildNodes(items []string) (nodes []Node, leafCount int) {
	// Track which directory strings have already produced a header node.
	dirSeen := make(map[string]bool)

	for _, item := range items {
		parts := strings.Split(item, "/")
		depth := len(parts) - 1

		switch depth {
		case 0:
			// Root-level leaf: no parent dir node needed.
			nodes = append(nodes, Node{
				Label:    parts[0],
				Type:     NodeTypeLeaf,
				Indent:   0,
				OrigPath: item,
				Selected: true,
			})
			leafCount++

		case 1:
			// One level deep: ensure a depth-0 dir header for parts[0].
			d1 := parts[0]
			if !dirSeen[d1] {
				dirSeen[d1] = true
				nodes = append(nodes, Node{
					Label:  d1 + "/",
					Type:   NodeTypeDir,
					Indent: 0,
				})
			}
			nodes = append(nodes, Node{
				Label:    parts[1],
				Type:     NodeTypeLeaf,
				Indent:   1,
				OrigPath: item,
				Selected: true,
			})
			leafCount++

		default:
			// Two or more levels deep: ensure depth-0 dir for parts[0] and
			// depth-1 dir for parts[0]/parts[1]. Deeper components are collapsed
			// into indent level 2, matching the bash script's behaviour.
			d1 := parts[0]
			d12 := path.Join(parts[0], parts[1])

			if !dirSeen[d1] {
				dirSeen[d1] = true
				nodes = append(nodes, Node{
					Label:  d1 + "/",
					Type:   NodeTypeDir,
					Indent: 0,
				})
			}
			if !dirSeen[d12] {
				dirSeen[d12] = true
				nodes = append(nodes, Node{
					Label:  parts[1] + "/",
					Type:   NodeTypeDir,
					Indent: 1,
				})
			}
			// The leaf label is the third component regardless of actual depth.
			nodes = append(nodes, Node{
				Label:    parts[2],
				Type:     NodeTypeLeaf,
				Indent:   2,
				OrigPath: item,
				Selected: true,
			})
			leafCount++
		}
	}

	return nodes, leafCount
}
