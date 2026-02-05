package tui

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/zulfikawr/warp/cmd/warp/help"
	"github.com/zulfikawr/warp/internal/config"
)

type fieldType int

const (
	fieldString fieldType = iota
	fieldInt
	fieldFloat
	fieldBool
)

type fieldState int

const (
	stateEditing fieldState = iota
	stateConfigHelp
)

type configField struct {
	label     string
	key       string // mapstructure key or just id
	fieldType fieldType
	input     textinput.Model
	boolVal   bool
}

type ConfigModel struct {
	fields     []*configField
	cursor     int
	width      int
	height     int
	statusMsg  string
	cfg        *config.Config
	state      fieldState
	helpScreen HelpScreen
}

func NewConfigModel() ConfigModel {
	cfg, err := config.LoadConfig()
	if err != nil {
		cfg = config.DefaultConfig()
	}

	m := ConfigModel{
		cfg:        cfg,
		width:      80,
		height:     24,
		helpScreen: NewHelpScreen("CONFIG HELP", help.ConfigHelpLines),
	}

	// Initialize fields matching internal/config/config.go
	m.fields = []*configField{
		{label: "Upload Directory", key: "upload_dir", fieldType: fieldString},
		{label: "Default Interface", key: "default_interface", fieldType: fieldString},
		{label: "Default Port", key: "default_port", fieldType: fieldInt},
		{label: "Buffer Size (bytes)", key: "buffer_size", fieldType: fieldInt},
		{label: "Max Upload (bytes)", key: "max_upload_size", fieldType: fieldInt}, // int64 logically, but field type int handles numbers
		{label: "Rate Limit (Mbps)", key: "rate_limit_mbps", fieldType: fieldFloat},
		{label: "Chunk Size (MB)", key: "chunk_size_mb", fieldType: fieldInt},
		{label: "Cache Size (MB)", key: "cache_size_mb", fieldType: fieldInt},
		{label: "Parallel Workers", key: "parallel_workers", fieldType: fieldInt},
		{label: "Disable Encryption", key: "no_encrypt", fieldType: fieldBool},
		{label: "Disable QR Code", key: "no_qr", fieldType: fieldBool},
		{label: "Disable Checksum", key: "no_checksum", fieldType: fieldBool},
		// Resume configuration
		{label: "Enable Resume", key: "enable_resume", fieldType: fieldBool},
		{label: "Auto Checkpoint (MB)", key: "auto_checkpoint_size_mb", fieldType: fieldInt},
		{label: "Checkpoint Interval (MB)", key: "checkpoint_interval_mb", fieldType: fieldInt},
		{label: "Checkpoint Expiry (hrs)", key: "checkpoint_expiry_hours", fieldType: fieldInt},
	}

	// Populate values
	for i, f := range m.fields {
		ti := textinput.New()
		ti.CharLimit = 100
		ti.Width = 30
		ti.Prompt = "" // We handle the cursor marker manually

		// Only the first field should be focused initially
		if i == 0 {
			ti.TextStyle = lipgloss.NewStyle().Foreground(PrimaryColor)
		} else {
			ti.TextStyle = lipgloss.NewStyle().Foreground(DimTextColor)
		}

		switch f.key {
		case "upload_dir":
			ti.SetValue(cfg.UploadDir)
		case "default_interface":
			ti.SetValue(cfg.DefaultInterface)
		case "default_port":
			ti.SetValue(strconv.Itoa(cfg.DefaultPort))
		case "buffer_size":
			ti.SetValue(strconv.Itoa(cfg.BufferSize))
		case "max_upload_size":
			ti.SetValue(strconv.FormatInt(cfg.MaxUploadSize, 10))
		case "rate_limit_mbps":
			ti.SetValue(fmt.Sprintf("%.1f", cfg.RateLimitMbps))
		case "chunk_size_mb":
			ti.SetValue(strconv.Itoa(cfg.ChunkSizeMB))
		case "cache_size_mb":
			ti.SetValue(strconv.FormatInt(cfg.CacheSizeMB, 10))
		case "parallel_workers":
			ti.SetValue(strconv.Itoa(cfg.ParallelWorkers))
		case "no_encrypt":
			f.boolVal = cfg.NoEncrypt
		case "no_qr":
			f.boolVal = cfg.NoQR
		case "no_checksum":
			f.boolVal = cfg.NoChecksum
		case "enable_resume":
			f.boolVal = cfg.EnableResume
		case "auto_checkpoint_size_mb":
			ti.SetValue(strconv.Itoa(cfg.AutoCheckpointSizeMB))
		case "checkpoint_interval_mb":
			ti.SetValue(strconv.Itoa(cfg.CheckpointIntervalMB))
		case "checkpoint_expiry_hours":
			ti.SetValue(strconv.Itoa(cfg.CheckpointExpiryHours))
		}
		f.input = ti
	}

	// Focus first field
	if len(m.fields) > 0 {
		m.fields[0].input.Focus()
	}

	return m
}

func (m *ConfigModel) SetHelp() {
	m.state = stateConfigHelp
}

func (m ConfigModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m ConfigModel) Update(msg tea.Msg) (ConfigModel, tea.Cmd, bool) {
	var cmd tea.Cmd
	quit := false

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch m.state {
		case stateConfigHelp:
			h, shouldExit := m.helpScreen.Update(msg)
			m.helpScreen = h
			if shouldExit {
				m.state = stateEditing
			}
			return m, nil, false

		case stateEditing:
			if msg.String() == "ctrl+h" {
				m.state = stateConfigHelp
				return m, nil, false
			}

			switch msg.String() {
			case "ctrl+c":
				return m, tea.Quit, false
			case "esc":
				return m, nil, true // Quit to home
			case "up", "shift+tab":
				m.cursor--
				if m.cursor < 0 {
					m.cursor = len(m.fields) - 1
				}
				m.focusField()
			case "down", "tab":
				m.cursor++
				if m.cursor >= len(m.fields) {
					m.cursor = 0
				}
				m.focusField()
			case "enter":
				m.cursor++
				if m.cursor >= len(m.fields) {
					m.cursor = 0
				}
				m.focusField()
			case " ":
				f := m.fields[m.cursor]
				if f.fieldType == fieldBool {
					f.boolVal = !f.boolVal
				}
			case "ctrl+s":
				m.saveConfig()
				m.statusMsg = "Saved!"
				return m, nil, false
			case "ctrl+r":
				m.resetToDefaults()
				m.statusMsg = "Reset to defaults!"
				return m, nil, false
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.helpScreen.SetSize(msg.Width, msg.Height)
	}

	if m.state == stateEditing {
		f := m.fields[m.cursor]
		if f.fieldType != fieldBool {
			f.input, cmd = f.input.Update(msg)
		}
	}

	return m, cmd, quit
}

func (m *ConfigModel) focusField() {
	for i, f := range m.fields {
		if i == m.cursor {
			f.input.Focus()
			// Reset text style when focused if needed
			f.input.TextStyle = lipgloss.NewStyle().Foreground(PrimaryColor)
		} else {
			f.input.Blur()
			f.input.TextStyle = lipgloss.NewStyle().Foreground(DimTextColor)
		}
	}
}

func (m *ConfigModel) saveConfig() {
	for _, f := range m.fields {
		switch f.key {
		case "upload_dir":
			m.cfg.UploadDir = f.input.Value()
		case "default_interface":
			m.cfg.DefaultInterface = f.input.Value()
		case "default_port":
			v, _ := strconv.Atoi(f.input.Value())
			m.cfg.DefaultPort = v
		case "buffer_size":
			v, _ := strconv.Atoi(f.input.Value())
			m.cfg.BufferSize = v
		case "max_upload_size":
			v, _ := strconv.ParseInt(f.input.Value(), 10, 64)
			m.cfg.MaxUploadSize = v
		case "rate_limit_mbps":
			v, _ := strconv.ParseFloat(f.input.Value(), 64)
			m.cfg.RateLimitMbps = v
		case "chunk_size_mb":
			v, _ := strconv.Atoi(f.input.Value())
			m.cfg.ChunkSizeMB = v
		case "cache_size_mb":
			v, _ := strconv.ParseInt(f.input.Value(), 10, 64)
			m.cfg.CacheSizeMB = v
		case "parallel_workers":
			v, _ := strconv.Atoi(f.input.Value())
			m.cfg.ParallelWorkers = v
		case "no_encrypt":
			m.cfg.NoEncrypt = f.boolVal
		case "no_qr":
			m.cfg.NoQR = f.boolVal
		case "no_checksum":
			m.cfg.NoChecksum = f.boolVal
		case "enable_resume":
			m.cfg.EnableResume = f.boolVal
		case "auto_checkpoint_size_mb":
			v, _ := strconv.Atoi(f.input.Value())
			m.cfg.AutoCheckpointSizeMB = v
		case "checkpoint_interval_mb":
			v, _ := strconv.Atoi(f.input.Value())
			m.cfg.CheckpointIntervalMB = v
		case "checkpoint_expiry_hours":
			v, _ := strconv.Atoi(f.input.Value())
			m.cfg.CheckpointExpiryHours = v
		}
	}

	// Validate interface exists if specified
	if m.cfg.DefaultInterface != "" {
		if err := m.validateInterface(m.cfg.DefaultInterface); err != nil {
			m.statusMsg = fmt.Sprintf("Warning: %v", err)
			return
		}
	}

	if err := config.SaveConfig(m.cfg); err != nil {
		m.statusMsg = fmt.Sprintf("Error saving: %v", err)
	}
}

func (m *ConfigModel) validateInterface(name string) error {
	interfaces, err := net.Interfaces()
	if err != nil {
		return fmt.Errorf("cannot list interfaces: %w", err)
	}

	for _, iface := range interfaces {
		if iface.Name == name {
			return nil
		}
	}

	return fmt.Errorf("interface '%s' not found (leave empty for auto-detect)", name)
}

func (m *ConfigModel) resetToDefaults() {
	// Reset to default config
	m.cfg = config.DefaultConfig()

	// Update all field values
	for _, f := range m.fields {
		switch f.key {
		case "upload_dir":
			f.input.SetValue(m.cfg.UploadDir)
		case "default_interface":
			f.input.SetValue(m.cfg.DefaultInterface)
		case "default_port":
			f.input.SetValue(strconv.Itoa(m.cfg.DefaultPort))
		case "buffer_size":
			f.input.SetValue(strconv.Itoa(m.cfg.BufferSize))
		case "max_upload_size":
			f.input.SetValue(strconv.FormatInt(m.cfg.MaxUploadSize, 10))
		case "rate_limit_mbps":
			f.input.SetValue(fmt.Sprintf("%.1f", m.cfg.RateLimitMbps))
		case "chunk_size_mb":
			f.input.SetValue(strconv.Itoa(m.cfg.ChunkSizeMB))
		case "cache_size_mb":
			f.input.SetValue(strconv.FormatInt(m.cfg.CacheSizeMB, 10))
		case "parallel_workers":
			f.input.SetValue(strconv.Itoa(m.cfg.ParallelWorkers))
		case "no_encrypt":
			f.boolVal = m.cfg.NoEncrypt
		case "no_qr":
			f.boolVal = m.cfg.NoQR
		case "no_checksum":
			f.boolVal = m.cfg.NoChecksum
		case "enable_resume":
			f.boolVal = m.cfg.EnableResume
		case "auto_checkpoint_size_mb":
			f.input.SetValue(strconv.Itoa(m.cfg.AutoCheckpointSizeMB))
		case "checkpoint_interval_mb":
			f.input.SetValue(strconv.Itoa(m.cfg.CheckpointIntervalMB))
		case "checkpoint_expiry_hours":
			f.input.SetValue(strconv.Itoa(m.cfg.CheckpointExpiryHours))
		}
	}
}

func (m ConfigModel) View() string {
	if m.state == stateConfigHelp {
		return m.helpScreen.View()
	}

	footerItems := []string{
		"[↑↓] Navigate",
		"[Space/Type] Edit",
		"[Ctrl+S] Save",
		"[Ctrl+R] Reset",
		"[Ctrl+H] Help",
		"[Esc] Back",
	}

	return RenderScreen("WARP CONFIG", m.width, m.height, footerItems, func(availWidth, availHeight int) string {
		var b strings.Builder

		labelStyle := GetLabelStyle()
		dimStyle := GetDimStyle()
		enabledStyle := GetEnabledStyle()
		disabledStyle := GetDisabledStyle()
		titleStyle := BoxTitleStyle

		// --- Dimensions ---
		boxWidth := GetStandardBoxWidth(m.width)

		// Subtract internal overhead:
		// Header(1) + Separator(1) = 2 lines
		overhead := 2

		maxVisible := availHeight - 2 - overhead // -2 for box border
		if maxVisible < 1 {
			maxVisible = 1
		}

		// --- Scrolling Logic ---
		start := 0
		if m.cursor >= maxVisible {
			start = m.cursor - maxVisible + 1
		}
		end := start + maxVisible
		if end > len(m.fields) {
			end = len(m.fields)
		}

		// --- Build Content (Fields) ---
		for i := start; i < end; i++ {
			f := m.fields[i]

			// Format: " [❯] Label        Value"
			marker := "  "
			label := dimStyle.Render(fmt.Sprintf("%-25s", f.label))

			if m.cursor == i {
				marker = labelStyle.Render("❯ ")
				label = labelStyle.Render(fmt.Sprintf("%-25s", f.label))
			}

			value := ""
			if f.fieldType == fieldBool {
				if f.boolVal {
					value = enabledStyle.Render("✓ ENABLED")
				} else {
					value = disabledStyle.Render("✗ DISABLED")
				}
			} else {
				value = f.input.View()
			}

			b.WriteString(fmt.Sprintf("%s%s %s\n", marker, label, value))
		}

		fieldContent := b.String()

		// --- Build Header & Footer ---
		// 1. Title & Status
		titleText := "Configuration"
		headerText := titleStyle.Render(titleText)

		if m.statusMsg != "" {
			// Justify Status to the right
			statusStyle := GetSuccessStyle().Bold(true)
			statusText := statusStyle.Render(m.statusMsg)

			// Available width for spacing
			innerWidth := boxWidth - 2
			gap := innerWidth - lipgloss.Width(headerText) - lipgloss.Width(statusText)
			if gap < 0 {
				gap = 0
			}
			headerText = headerText + strings.Repeat(" ", gap) + statusText
		}

		// 2. Separator
		// Calculate effective width inside the box (width - 2 border)
		separatorWidth := boxWidth - 2
		separator := lipgloss.NewStyle().Foreground(PrimaryColor).Render(strings.Repeat("-", separatorWidth))

		// --- Assemble Box ---
		// Use JoinVertical to stack: Header -> Separator -> Fields
		innerContent := lipgloss.JoinVertical(
			lipgloss.Left,
			headerText,
			separator,
			fieldContent,
		)

		// Render the final box
		box := BoxStyle.Width(boxWidth).Render(innerContent)

		// Center everything on screen
		return lipgloss.Place(m.width-2, availHeight, lipgloss.Center, lipgloss.Center, box)
	})
}
