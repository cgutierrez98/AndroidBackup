package main

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

type midnightTheme struct{}

var _ fyne.Theme = (*midnightTheme)(nil)

func (m midnightTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	switch name {
	case theme.ColorNameBackground:
		return color.RGBA{0x1E, 0x1E, 0x2E, 0xFF} // Main Background
	case theme.ColorNameButton:
		return color.RGBA{0x89, 0xB4, 0xFA, 0xFF} // Primary Action (Blue)
	case theme.ColorNameDisabledButton:
		return color.RGBA{0x58, 0x5B, 0x70, 0xFF} // Lighter Disabled Gray
	case theme.ColorNameDisabled:
		return color.RGBA{0x7F, 0x84, 0x9C, 0xFF} // Brighter Disabled Text
	case theme.ColorNameError:
		return color.RGBA{0xF3, 0x8B, 0xA8, 0xFF} // Red
	case theme.ColorNameForeground:
		return color.RGBA{0xFF, 0xFF, 0xFF, 0xFF} // Pure White Text
	case theme.ColorNameHover:
		return color.RGBA{0x31, 0x32, 0x44, 0xFF} // Hover overlay
	case theme.ColorNameInputBackground:
		return color.RGBA{0x11, 0x11, 0x1B, 0xFF} // Darker Input BG
	case theme.ColorNamePlaceHolder:
		return color.RGBA{0xBA, 0xC2, 0xDE, 0xFF} // Brighter Placeholder
	case theme.ColorNamePressed:
		return color.RGBA{0x11, 0x11, 0x1B, 0xFF} // Pressed
	case theme.ColorNamePrimary:
		return color.RGBA{0x89, 0xB4, 0xFA, 0xFF} // Primary Color
	case theme.ColorNameScrollBar:
		return color.RGBA{0x45, 0x47, 0x5A, 0xFF}
	case theme.ColorNameShadow:
		return color.RGBA{0x00, 0x00, 0x00, 0x66}
	case theme.ColorNameSelection:
		return color.RGBA{0x45, 0x47, 0x5A, 0xFF} // Dropdown selection highlight
	case theme.ColorNameFocus:
		return color.RGBA{0x89, 0xB4, 0xFA, 0xFF} // Focus ring color
	case theme.ColorNameMenuBackground:
		return color.RGBA{0x2A, 0x2A, 0x3C, 0xFF} // Lighter background for dropdowns
	case theme.ColorNameOverlayBackground:
		return color.RGBA{0x2A, 0x2A, 0x3C, 0xFF} // Popup overlay
	case theme.ColorNameSeparator:
		return color.RGBA{0x45, 0x47, 0x5A, 0xFF}
	default:
		return theme.DefaultTheme().Color(name, variant)
	}
}

func (m midnightTheme) Font(style fyne.TextStyle) fyne.Resource {
	return theme.DefaultTheme().Font(style)
}

func (m midnightTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(name)
}

func (m midnightTheme) Size(name fyne.ThemeSizeName) float32 {
	return theme.DefaultTheme().Size(name)
}
