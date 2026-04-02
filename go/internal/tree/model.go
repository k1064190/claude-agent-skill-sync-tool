// ABOUTME: Bubbletea Model implementing the hierarchical tree TUI for item selection.
// ABOUTME: Mirrors the bash tui_tree_select function with keyboard navigation and preview panel.

package tree

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// DescFunc is the signature for a callback that returns a human-readable
// description string for a given item's relative path.
//
// Args:
//
//	relPath (string): Relative path of the item (e.g. "business/pm.md").
//
// Returns:
//
//	description (string): Human-readable description, may be empty.
type DescFunc func(relPath string) string

// Model is the bubbletea Model for the hierarchical tree TUI.
// It holds display nodes, cursor/scroll state, and the optional description
// callback for the preview panel. After the program exits, Confirmed and
// SelectedPaths carry the user's choices.
type Model struct {
	// nodes is the flat ordered list of display nodes produced by BuildNodes.
	nodes []Node
	// leafCount is the total number of leaf nodes across all nodes.
	leafCount int
	// descFn is an optional callback returning descriptions for leaf OrigPaths.
	descFn DescFunc

	// cursor is the display index of the currently highlighted row.
	cursor int
	// scrollTop is the first visible display index.
	scrollTop int
	// previewOpen indicates whether the description preview panel is shown.
	previewOpen bool

	// termWidth and termHeight hold the terminal dimensions.
	termWidth  int
	termHeight int

	// done is true once the user has pressed Enter or q/Esc.
	done bool
	// Confirmed is true if the user pressed Enter (as opposed to q/Esc).
	Confirmed bool
	// SelectedPaths contains the OrigPath of every selected leaf node after
	// the program exits.
	SelectedPaths []string
}

// NewModel constructs a Model from a sorted item list and an optional
// description callback. All leaves start pre-selected. Terminal dimensions
// start at conservative defaults; bubbletea sends a WindowSizeMsg almost
// immediately and the model updates before the first meaningful render.
//
// Args:
//
//	items  ([]string): Sorted relative item paths.
//	descFn (DescFunc): Optional; pass nil to disable the preview panel.
//
// Returns:
//
//	m (Model): Initialised Model ready for use with bubbletea.
func NewModel(items []string, descFn DescFunc) Model {
	nodes, leafCount := BuildNodes(items)
	return Model{
		nodes:       nodes,
		leafCount:   leafCount,
		descFn:      descFn,
		termWidth:   80,
		termHeight:  24,
		cursor:      0,
		scrollTop:   0,
		previewOpen: false,
	}
}

// Init satisfies the bubbletea.Model interface. No I/O commands are needed at
// startup.
//
// Returns:
//
//	cmd (tea.Cmd): Always nil.
func (m Model) Init() tea.Cmd {
	return nil
}

// Update satisfies the bubbletea.Model interface. It processes key messages
// and returns an updated Model plus any follow-up command.
//
// Args:
//
//	msg (tea.Msg): The incoming bubbletea message.
//
// Returns:
//
//	model (tea.Model): Updated Model.
//	cmd   (tea.Cmd):   Follow-up command, or nil.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	prevScroll := m.scrollTop

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				if m.cursor < m.scrollTop {
					m.scrollTop--
				}
			}

		case "down", "j":
			if m.cursor < len(m.nodes)-1 {
				m.cursor++
				visRows := m.visibleRows()
				if m.cursor >= m.scrollTop+visRows {
					m.scrollTop++
				}
			}

		case "right":
			if m.descFn != nil {
				m.previewOpen = true
			}

		case "left":
			m.previewOpen = false

		case " ":
			m.toggleCurrent()

		case "a", "A":
			for i := range m.nodes {
				if m.nodes[i].Type == NodeTypeLeaf {
					m.nodes[i].Selected = true
				}
			}

		case "n", "N":
			for i := range m.nodes {
				if m.nodes[i].Type == NodeTypeLeaf {
					m.nodes[i].Selected = false
				}
			}

		case "enter":
			m.Confirmed = true
			m.done = true
			m.collectSelected()
			return m, tea.Quit

		case "q", "Q", "esc":
			m.Confirmed = false
			m.done = true
			return m, tea.Quit
		}

	case tea.WindowSizeMsg:
		m.termWidth = msg.Width
		m.termHeight = msg.Height
		// Clamp scroll so the cursor stays visible after resize.
		m.ensureCursorVisible()
		return m, tea.ClearScreen
	}

	// Force full repaint when viewport scrolls — bubbletea's diff renderer
	// can desync over SSH when many lines shift simultaneously.
	if m.scrollTop != prevScroll {
		return m, tea.ClearScreen
	}

	return m, nil
}

// toggleCurrent toggles the selection state of the node at m.cursor.
// For dir nodes it cascades the toggle to all descendant leaves:
// if all are currently selected → deselect all; otherwise → select all.
// For leaf nodes it simply flips the Selected bit.
func (m *Model) toggleCurrent() {
	cur := &m.nodes[m.cursor]
	if cur.Type == NodeTypeDir {
		descendants := m.leafDescendants(m.cursor)
		allSelected := true
		for _, idx := range descendants {
			if !m.nodes[idx].Selected {
				allSelected = false
				break
			}
		}
		newVal := !allSelected
		for _, idx := range descendants {
			m.nodes[idx].Selected = newVal
		}
	} else {
		cur.Selected = !cur.Selected
	}
}

// leafDescendants returns the display indices of all leaf nodes that are
// direct or indirect children of the dir node at position dirIdx. Scanning
// stops when a node at the same or shallower indent level is encountered.
//
// Args:
//
//	dirIdx (int): Index into m.nodes of the directory node.
//
// Returns:
//
//	indices ([]int): Display indices of descendant leaf nodes.
func (m *Model) leafDescendants(dirIdx int) []int {
	dirIndent := m.nodes[dirIdx].Indent
	var result []int
	for i := dirIdx + 1; i < len(m.nodes); i++ {
		if m.nodes[i].Indent <= dirIndent {
			break
		}
		if m.nodes[i].Type == NodeTypeLeaf {
			result = append(result, i)
		}
	}
	return result
}

// dirState returns the checkbox display state for a dir node based on the
// selection state of its descendant leaves.
//
// Args:
//
//	dirIdx (int): Index into m.nodes of the directory node.
//
// Returns:
//
//	state (string): One of "checked", "unchecked", or "partial".
func (m *Model) dirState(dirIdx int) string {
	descendants := m.leafDescendants(dirIdx)
	if len(descendants) == 0 {
		return "unchecked"
	}
	selected := 0
	for _, idx := range descendants {
		if m.nodes[idx].Selected {
			selected++
		}
	}
	switch selected {
	case 0:
		return "unchecked"
	case len(descendants):
		return "checked"
	default:
		return "partial"
	}
}

// selectedLeafCount returns the number of currently selected leaf nodes.
//
// Returns:
//
//	count (int): Number of selected leaves.
func (m *Model) selectedLeafCount() int {
	count := 0
	for i := range m.nodes {
		if m.nodes[i].Type == NodeTypeLeaf && m.nodes[i].Selected {
			count++
		}
	}
	return count
}

// collectSelected populates m.SelectedPaths with the OrigPath of every
// selected leaf node, preserving display order.
func (m *Model) collectSelected() {
	m.SelectedPaths = nil
	for i := range m.nodes {
		if m.nodes[i].Type == NodeTypeLeaf && m.nodes[i].Selected {
			m.SelectedPaths = append(m.SelectedPaths, m.nodes[i].OrigPath)
		}
	}
}

// ensureCursorVisible adjusts scrollTop so the cursor row is within the
// visible viewport. Called after terminal resize or any cursor movement.
func (m *Model) ensureCursorVisible() {
	vis := m.visibleRows()
	if m.cursor < m.scrollTop {
		m.scrollTop = m.cursor
	}
	if m.cursor >= m.scrollTop+vis {
		m.scrollTop = m.cursor - vis + 1
	}
	if m.scrollTop < 0 {
		m.scrollTop = 0
	}
}

// previewHeight returns the number of lines reserved for the preview panel.
// Returns 0 when no descFn was provided (preview disabled).
//
// Returns:
//
//	height (int): Number of lines for the preview area (0 or 5).
func (m *Model) previewHeight() int {
	if m.descFn == nil {
		return 0
	}
	return 5
}

// visibleRows returns the number of item rows that fit between the header,
// scroll indicators, and optional preview panel.
//
// Returns:
//
//	rows (int): Visible item row count (minimum 3).
func (m *Model) visibleRows() int {
	// Layout: 1 header + 1 scroll-up indicator + <vis> items + 1 scroll-down + previewHeight
	rows := m.termHeight - 3 - m.previewHeight()
	if rows > len(m.nodes) {
		rows = len(m.nodes)
	}
	if rows < 3 {
		rows = 3
	}
	return rows
}

// View satisfies the bubbletea.Model interface. It builds and returns the
// complete TUI frame as a single string. Bubbletea writes the entire frame
// atomically, eliminating partial-render flicker.
//
// Returns:
//
//	frame (string): Complete terminal frame including ANSI escape codes.
func (m Model) View() string {
	if m.done {
		// Return an empty frame so bubbletea clears the TUI on exit.
		return ""
	}

	var sb strings.Builder
	visRows := m.visibleRows()
	n := len(m.nodes)
	selCount := m.selectedLeafCount()

	// ANSI codes.
	const (
		reset  = "\033[0m"
		rev    = "\033[7m"
		green  = "\033[32m"
		yellow = "\033[33m"
		cyan   = "\033[36m"
		bold   = "\033[1m"
		dim    = "\033[2m"
	)

	// Header line.
	hint := ""
	if m.descFn != nil {
		hint = "  \u2192=preview"
	}
	sb.WriteString(fmt.Sprintf(
		"%s%s [%d/%d]  \u2191\u2193=navigate  Space=toggle  a=all  n=none  Enter=confirm  q=cancel%s%s\n",
		bold, cyan, selCount, m.leafCount, hint, reset,
	))

	// Scroll-up indicator.
	if m.scrollTop > 0 {
		sb.WriteString(fmt.Sprintf("  %s\u2191 %d more above%s\n", yellow, m.scrollTop, reset))
	} else {
		sb.WriteString("\n")
	}

	// Visible item rows.
	end := m.scrollTop + visRows
	if end > n {
		end = n
	}

	for i := m.scrollTop; i < end; i++ {
		node := &m.nodes[i]

		// Build indentation prefix.
		indent := strings.Repeat("  ", node.Indent)

		// Determine checkbox mark and colour.
		var mark, colour string
		if node.Type == NodeTypeDir {
			state := m.dirState(i)
			switch state {
			case "checked":
				mark = "[x]"
				colour = green
			case "partial":
				mark = "[~]"
				colour = yellow
			default:
				mark = "[ ]"
				colour = ""
			}
		} else {
			if node.Selected {
				mark = "[x]"
				colour = green
			} else {
				mark = "[ ]"
				colour = ""
			}
		}

		// Label width: total width minus fixed chrome (cursor " ▶ " = 3 chars,
		// mark "[x]" = 3, space = 1, indent, trailing space = 1 → 8 + indent).
		labelWidth := m.termWidth - 8 - len(indent)
		if labelWidth < 1 {
			labelWidth = 1
		}

		if i == m.cursor {
			// Current row: reverse video, triangle cursor.
			sb.WriteString(fmt.Sprintf(
				"%s \u25b6 %s%s %-*s %s\n",
				rev, indent, mark, labelWidth, node.Label, reset,
			))
		} else {
			sb.WriteString(fmt.Sprintf(
				"   %s%s%s%s %s\n",
				colour, indent, mark, reset, node.Label,
			))
		}
	}

	// Pad remaining visible rows.
	for i := end; i < m.scrollTop+visRows; i++ {
		sb.WriteString("\n")
	}

	// Scroll-down indicator.
	if m.scrollTop+visRows < n {
		sb.WriteString(fmt.Sprintf("  %s\u2193 %d more below%s\n", yellow, n-m.scrollTop-visRows, reset))
	} else {
		sb.WriteString("\n")
	}

	// Preview panel (5 lines when a descFn is provided).
	if m.descFn != nil {
		if m.previewOpen {
			// Separator line.
			sb.WriteString(fmt.Sprintf("%s%s\u2500\u2500 description \u2500\u2500%s\n", bold, cyan, reset))

			// Resolve description text.
			var descText string
			cur := &m.nodes[m.cursor]
			if cur.Type == NodeTypeDir {
				descs := m.leafDescendants(m.cursor)
				descText = fmt.Sprintf("%d items in %s", len(descs), cur.Label)
			} else {
				descText = m.descFn(cur.OrigPath)
				if descText == "" {
					descText = "(no description)"
				}
			}

			// Word-wrap to terminal width minus 3.
			wrapWidth := m.termWidth - 3
			if wrapWidth < 10 {
				wrapWidth = 10
			}
			lines := wrapText(descText, wrapWidth)

			// Print up to 3 lines, padding the rest.
			for row := 0; row < 3; row++ {
				if row < len(lines) {
					sb.WriteString(fmt.Sprintf(" %s\n", lines[row]))
				} else {
					sb.WriteString("\n")
				}
			}
			sb.WriteString("\n") // blank line to complete the 5-line block
		} else {
			// Hint when preview is closed.
			sb.WriteString(fmt.Sprintf("  %s\u2192 right arrow to preview description%s\n", dim, reset))
			// Pad remaining 4 lines.
			for i := 1; i < 5; i++ {
				sb.WriteString("\n")
			}
		}
	}

	return sb.String()
}

// wrapText breaks text into lines of at most width runes, splitting on spaces
// where possible (word-wrap). It replicates the behaviour of `fold -s -w width`.
//
// Args:
//
//	text  (string): The text to wrap.
//	width (int):    Maximum line width in characters.
//
// Returns:
//
//	lines ([]string): Wrapped lines with no trailing newlines.
func wrapText(text string, width int) []string {
	if width <= 0 || text == "" {
		return []string{text}
	}

	var lines []string
	for len(text) > 0 {
		if len(text) <= width {
			lines = append(lines, text)
			break
		}

		// Find the last space within the width boundary.
		cutAt := width
		if spaceIdx := strings.LastIndex(text[:width], " "); spaceIdx > 0 {
			cutAt = spaceIdx
		}

		lines = append(lines, text[:cutAt])
		text = strings.TrimLeft(text[cutAt:], " ")
	}

	return lines
}
