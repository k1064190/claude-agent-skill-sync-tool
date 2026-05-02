package ui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/k1064190/claude-agent-skill-sync-tool/go/internal/config"
)

type PlatformModel struct {
	choices   []config.Platform
	cursor    int
	selected  map[int]struct{}
	Confirmed bool
}

func NewPlatformModel() PlatformModel {
	return PlatformModel{
		choices:  config.AllPlatforms(),
		selected: make(map[int]struct{}),
	}
}

func (m PlatformModel) Init() tea.Cmd {
	return nil
}

func (m PlatformModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.choices)-1 {
				m.cursor++
			}
		case "enter":
			m.Confirmed = true
			return m, tea.Quit
		case " ":
			_, ok := m.selected[m.cursor]
			if ok {
				delete(m.selected, m.cursor)
			} else {
				m.selected[m.cursor] = struct{}{}
			}
		case "a":
			for i := range m.choices {
				m.selected[i] = struct{}{}
			}
		case "n":
			m.selected = make(map[int]struct{})
		}
	}
	return m, nil
}

func (m PlatformModel) View() string {
	s := "\n  Which platforms would you like to sync to?\n\n"
	for i, choice := range m.choices {
		cursor := " "
		if m.cursor == i {
			cursor = ">"
		}
		checked := " "
		if _, ok := m.selected[i]; ok {
			checked = "x"
		}
		s += fmt.Sprintf("  %s [%s] %s\n", cursor, checked, choice)
	}
	s += "\n  (Press Space to toggle, Enter to confirm, a to select all, n to clear)\n\n"
	return s
}

func (m PlatformModel) SelectedPlatforms() []config.Platform {
	var platforms []config.Platform
	for i, choice := range m.choices {
		if _, ok := m.selected[i]; ok {
			platforms = append(platforms, choice)
		}
	}
	return platforms
}

func RunPlatformSelect() ([]config.Platform, error) {
	m := NewPlatformModel()
	p := tea.NewProgram(m)
	finalModel, err := p.Run()
	if err != nil {
		return nil, err
	}
	result, ok := finalModel.(PlatformModel)
	if !ok || !result.Confirmed {
		return nil, nil // Cancelled
	}
	return result.SelectedPlatforms(), nil
}
