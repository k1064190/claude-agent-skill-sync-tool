// ABOUTME: Parses YAML frontmatter from markdown files to extract the description field.
// ABOUTME: Implements the same logic as the awk command in the bash _tui_yaml_desc function.

package yaml

import (
	"bufio"
	"os"
	"strings"
)

// ExtractDescription reads a markdown file and extracts the value of the
// "description:" key from the YAML frontmatter block (between the first pair
// of "---" delimiters). It replicates the behaviour of the awk one-liner used
// in the bash scripts:
//
//	awk '/^---$/{c++; next} c==1 && /^description:/{sub(/^description:[[:space:]]*/,""); print; exit}'
//
// Args:
//
//	filePath (string): Absolute or relative path to the markdown file.
//
// Returns:
//
//	description (string): The trimmed description value, or "(no description)"
//	                       when the file cannot be read or the key is absent.
func ExtractDescription(filePath string) string {
	f, err := os.Open(filePath)
	if err != nil {
		return "(file not found)"
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	dashCount := 0

	for scanner.Scan() {
		line := scanner.Text()

		if line == "---" {
			dashCount++
			// Stop scanning once we leave the first frontmatter block.
			if dashCount == 2 {
				break
			}
			continue
		}

		// Only inspect lines inside the first frontmatter block.
		if dashCount == 1 {
			const prefix = "description:"
			if strings.HasPrefix(line, prefix) {
				value := strings.TrimPrefix(line, prefix)
				value = strings.TrimLeft(value, " \t")
				if value != "" {
					return value
				}
			}
		}
	}

	return "(no description)"
}
