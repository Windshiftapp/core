package styles

// terminalColors is the subset of a standard 16-color terminal scheme used
// by the TUI's semantic palette. Base colors come from Alacritty themes;
// muted text colors are lifted for readable hints on dark backgrounds.
type terminalColors struct {
	background, foreground   string
	muted                    string
	selection, selectionText string
	black, brightBlack       string
	red, green, yellow       string
	blue, brightBlue         string
	magenta, cyan            string
}

func terminalPalette(c terminalColors) Palette {
	return Palette{
		Primary:          hex(c.blue),
		PrimaryHovered:   hex(c.brightBlue),
		PrimarySubtle:    hex(c.selection),
		Accent:           hex(c.magenta),
		FgBase:           hex(c.foreground),
		FgSubtle:         hex(c.foreground),
		FgMuted:          hex(c.muted),
		FgInverse:        hex(c.background),
		BgBase:           hex(c.background),
		BgSurface:        hex(c.background),
		BgSurfaceHovered: hex(c.selection),
		BgOverlay:        hex(c.background),
		Border:           hex(c.brightBlack),
		BorderSubtle:     hex(c.black),
		BorderFocus:      hex(c.brightBlue),
		Success:          hex(c.green),
		Warning:          hex(c.yellow),
		Danger:           hex(c.red),
		Info:             hex(c.cyan),
		Selected:         hex(c.selection),
		OnPrimary:        hex(c.selectionText),
		GradFrom:         hex(c.blue),
		GradTo:           hex(c.magenta),
	}
}

func CatppuccinMocha() Palette {
	return terminalPalette(terminalColors{
		muted: "#a6adc8", background: "#1e1e2e", foreground: "#cdd6f4", selection: "#353748", selectionText: "#cdd6f4",
		black: "#45475a", brightBlack: "#585b70", red: "#f38ba8", green: "#a6e3a1", yellow: "#f9e2af",
		blue: "#89b4fa", brightBlue: "#74a8fc", magenta: "#f5c2e7", cyan: "#94e2d5",
	})
}

func CatppuccinFrappe() Palette {
	return terminalPalette(terminalColors{
		muted: "#a5adce", background: "#303446", foreground: "#c6d0f5", selection: "#44495d", selectionText: "#c6d0f5",
		black: "#51576d", brightBlack: "#626880", red: "#e78284", green: "#a6d189", yellow: "#e5c890",
		blue: "#8caaee", brightBlue: "#7b9ef0", magenta: "#f4b8e4", cyan: "#81c8be",
	})
}

func Dracula() Palette {
	return terminalPalette(terminalColors{
		muted: "#a2a8bf", background: "#282a36", foreground: "#f8f8f2", selection: "#44475a", selectionText: "#f8f8f2",
		black: "#21222c", brightBlack: "#6272a4", red: "#ff5555", green: "#50fa7b", yellow: "#f1fa8c",
		blue: "#bd93f9", brightBlue: "#d6acff", magenta: "#ff79c6", cyan: "#8be9fd",
	})
}

func Nord() Palette {
	return terminalPalette(terminalColors{
		muted: "#a5afc1", background: "#2e3440", foreground: "#d8dee9", selection: "#3f4758", selectionText: "#d8dee9",
		black: "#3b4252", brightBlack: "#4c566a", red: "#bf616a", green: "#a3be8c", yellow: "#ebcb8b",
		blue: "#81a1c1", brightBlue: "#81a1c1", magenta: "#b48ead", cyan: "#88c0d0",
	})
}

func GruvboxDark() Palette {
	return terminalPalette(terminalColors{
		muted: "#bdae93", background: "#282828", foreground: "#ebdbb2", selection: "#ebdbb2", selectionText: "#282828",
		black: "#282828", brightBlack: "#928374", red: "#fb4934", green: "#b8bb26", yellow: "#fabd2f",
		blue: "#458588", brightBlue: "#83a598", magenta: "#d3869b", cyan: "#8ec07c",
	})
}

func TokyoNight() Palette {
	return terminalPalette(terminalColors{
		muted: "#9aa5ce", background: "#1a1b26", foreground: "#c0caf5", selection: "#283457", selectionText: "#c0caf5",
		black: "#15161e", brightBlack: "#414868", red: "#f7768e", green: "#9ece6a", yellow: "#e0af68",
		blue: "#7aa2f7", brightBlue: "#7aa2f7", magenta: "#bb9af7", cyan: "#7dcfff",
	})
}

func KanagawaWave() Palette {
	return terminalPalette(terminalColors{
		muted: "#a6a69c", background: "#1f1f28", foreground: "#dcd7ba", selection: "#2d4f67", selectionText: "#c8c093",
		black: "#16161d", brightBlack: "#727169", red: "#e82424", green: "#98bb6c", yellow: "#e6c384",
		blue: "#7e9cd8", brightBlue: "#7fb4ca", magenta: "#957fb8", cyan: "#7aa89f",
	})
}

func RosePine() Palette {
	return terminalPalette(terminalColors{
		muted: "#a5a1b8", background: "#1f1d2e", foreground: "#e0def4", selection: "#2f2c40", selectionText: "#e0def4",
		black: "#26233a", brightBlack: "#908caa", red: "#eb6f92", green: "#31748f", yellow: "#f6c177",
		blue: "#9ccfd8", brightBlue: "#9ccfd8", magenta: "#c4a7e7", cyan: "#ebbcba",
	})
}

func EverforestDark() Palette {
	return terminalPalette(terminalColors{
		muted: "#a7b0a0", background: "#2d353b", foreground: "#d3c6aa", selection: "#414b51", selectionText: "#d3c6aa",
		black: "#343f44", brightBlack: "#859289", red: "#e67e80", green: "#a7c080", yellow: "#dbbc7f",
		blue: "#7fbbb3", brightBlue: "#7fbbb3", magenta: "#d699b6", cyan: "#83c092",
	})
}

func SolarizedDark() Palette {
	return terminalPalette(terminalColors{
		muted: "#839496", background: "#002b36", foreground: "#839496", selection: "#073642", selectionText: "#93a1a1",
		black: "#073642", brightBlack: "#586e75", red: "#dc322f", green: "#859900", yellow: "#b58900",
		blue: "#268bd2", brightBlue: "#839496", magenta: "#d33682", cyan: "#2aa198",
	})
}

func OneDark() Palette {
	return terminalPalette(terminalColors{
		muted: "#9da5b4", background: "#282c34", foreground: "#abb2bf", selection: "#abb2bf", selectionText: "#282c34",
		black: "#1e2127", brightBlack: "#5c6370", red: "#e06c75", green: "#98c379", yellow: "#d19a66",
		blue: "#61afef", brightBlue: "#61afef", magenta: "#c678dd", cyan: "#56b6c2",
	})
}

// WindshiftDark preserves source compatibility while old persisted theme
// names are migrated to Catppuccin Mocha by ByName.
func WindshiftDark() Palette { return CatppuccinMocha() }
