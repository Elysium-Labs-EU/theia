// Package ui defines terminal color palette and lipgloss styles for theia CLI output.
package ui

import "github.com/charmbracelet/lipgloss"

var (
	TextMuted   = lipgloss.NewStyle().Faint(true)                        // hints, next-step lines
	TextCommand = lipgloss.NewStyle().Bold(true).Foreground(ColorAccent) // theia system update
	TextBold    = lipgloss.NewStyle().Bold(true)

	LabelSuccess = lipgloss.NewStyle().Bold(true).Foreground(ColorSuccess)
	LabelWarning = lipgloss.NewStyle().Bold(true).Foreground(ColorWarning)
	LabelError   = lipgloss.NewStyle().Bold(true).Foreground(ColorError)
	LabelInfo    = lipgloss.NewStyle().Bold(true).Foreground(ColorInfo)

	TableBorderColor = ColorMuted
	TableHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(ColorAccent).
				Padding(0, 1)

	TableCellStyle = lipgloss.NewStyle().Padding(0, 1)

	TableEvenRowStyle = TableCellStyle
	TableOddRowStyle  = TableCellStyle.Faint(true)
	TableMutedStyle   = TableCellStyle.Faint(true)
)
