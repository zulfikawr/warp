package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/zulfikawr/warp/internal/ui"
)

type FileEntry struct {
	Name     string
	IsDir    bool
	Selected bool
	Size     int64
	ModTime  time.Time
}

type SortMode int

const (
	SortDefault  SortMode = iota // Folders first, then A-Z
	SortName                     // A-Z (mixed)
	SortNameDesc                 // Z-A (mixed)
	SortNewest                   // Date Desc
	SortOldest                   // Date Asc
	SortSizeDesc                 // Size Desc
	SortSizeAsc                  // Size Asc
)

func (s SortMode) String() string {
	switch s {
	case SortDefault:
		return "[ ⇅ ] Default"
	case SortName:
		return "[ A-Z ] Name (Asc)"
	case SortNameDesc:
		return "[ Z-A ] Name (Desc)"
	case SortNewest:
		return "[ ↓ ] Newest"
	case SortOldest:
		return "[ ↑ ] Oldest"
	case SortSizeDesc:
		return "[ ▼ ] Size (Lrg)"
	case SortSizeAsc:
		return "[ ▲ ] Size (Sml)"
	default:
		return "[ ? ] Unknown"
	}
}

type FilePicker struct {
	Dir          string
	Entries      []FileEntry
	Cursor       int
	Offset       int
	Width        int
	Height       int
	SortMode     SortMode
	Err          error
	DirOnly      bool
	Title        string
	CustomFooter []string
}

func NewFilePicker(startDir string) FilePicker {
	fp := FilePicker{
		Dir:      startDir,
		SortMode: SortDefault,
	}
	fp.LoadEntries()
	return fp
}

func (fp *FilePicker) LoadEntries() {
	entries, err := os.ReadDir(fp.Dir)
	var list []FileEntry

	// Add ".." for parent directory if not root
	if fp.Dir != "/" && fp.Dir != filepath.VolumeName(fp.Dir)+string(os.PathSeparator) {
		list = append(list, FileEntry{Name: "..", IsDir: true})
	}

	if err == nil {
		var content []FileEntry
		for _, e := range entries {
			if fp.DirOnly && !e.IsDir() {
				continue
			}
			fe := FileEntry{Name: e.Name(), IsDir: e.IsDir()}
			info, err := e.Info()
			if err == nil {
				fe.Size = info.Size()
				fe.ModTime = info.ModTime()
			}
			content = append(content, fe)
		}

		// Sort logic
		sort.Slice(content, func(i, j int) bool {
			a, b := content[i], content[j]

			// For default sort, folders always come first
			if fp.SortMode == SortDefault {
				if a.IsDir != b.IsDir {
					return a.IsDir
				}
				return strings.ToLower(a.Name) < strings.ToLower(b.Name)
			}

			switch fp.SortMode {
			case SortName:
				return strings.ToLower(a.Name) < strings.ToLower(b.Name)
			case SortNewest:
				return a.ModTime.After(b.ModTime)
			case SortOldest:
				return a.ModTime.Before(b.ModTime)
			case SortSizeDesc:
				return a.Size > b.Size
			case SortSizeAsc:
				return a.Size < b.Size
			default:
				return strings.ToLower(a.Name) < strings.ToLower(b.Name)
			}
		})

		list = append(list, content...)
	} else {
		fp.Err = err
	}
	fp.Entries = list
	fp.Cursor = 0
	fp.Offset = 0
}

func (fp *FilePicker) Update(msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if fp.Cursor > 0 {
				fp.Cursor--
			}
		case "down", "j":
			if fp.Cursor < len(fp.Entries)-1 {
				fp.Cursor++
			}
		case "left", "h":
			parent := filepath.Dir(fp.Dir)
			if parent != fp.Dir {
				fp.Dir = parent
				fp.LoadEntries()
			}
		case "right", "l":
			if fp.Cursor < len(fp.Entries) {
				entry := fp.Entries[fp.Cursor]
				if entry.Name == ".." {
					parent := filepath.Dir(fp.Dir)
					fp.Dir = parent
					fp.LoadEntries()
				} else if entry.IsDir {
					fp.Dir = filepath.Join(fp.Dir, entry.Name)
					fp.LoadEntries()
				}
			}
		case " ":
			if fp.Cursor < len(fp.Entries) && fp.Entries[fp.Cursor].Name != ".." {
				if fp.DirOnly {
					// Clear other selections for single select mode
					for i := range fp.Entries {
						fp.Entries[i].Selected = false
					}
					fp.Entries[fp.Cursor].Selected = true
				} else {
					fp.Entries[fp.Cursor].Selected = !fp.Entries[fp.Cursor].Selected
				}
			}
		case "enter":
			return nil, true
		case "s":
			fp.SortMode = (fp.SortMode + 1) % 6
			fp.LoadEntries()
		}
	}

	reservedLines := 8
	availHeight := max(fp.Height-reservedLines, 1)

	if fp.Cursor < fp.Offset {
		fp.Offset = fp.Cursor
	} else if fp.Cursor >= fp.Offset+availHeight {
		fp.Offset = fp.Cursor - availHeight + 1
	}

	return nil, false
}

func (fp FilePicker) View() string {
	title := "WARP SEND"
	if fp.Title != "" {
		title = fp.Title
	}

	footerItems := []string{
		"[↑↓] Navigate",
		"[Space] Select",
		"[⏎] Send",
		"[S] Sort",
		"[?] Help",
		"[Esc] Back",
	}

	if len(fp.CustomFooter) > 0 {
		footerItems = append(footerItems, fp.CustomFooter...)
	}

	return RenderScreen(title, fp.Width, fp.Height, footerItems, func(availWidth, availHeight int) string {
		w := availWidth

		boxWidth := w - 4
		if boxWidth < 40 {
			boxWidth = 40
		}

		labelStyle := lipgloss.NewStyle().Foreground(PrimaryColor).Bold(true)
		selectedStyle := MenuItemSelectedStyle
		normalStyle := MenuItemStyle

		dirLabel := fmt.Sprintf("%s", fp.Dir)
		sortLabel := lipgloss.NewStyle().Foreground(DimTextColor).Render(fp.SortMode.String())

		headerRow := lipgloss.JoinHorizontal(lipgloss.Top,
			labelStyle.Render(dirLabel),
			lipgloss.PlaceHorizontal(boxWidth-len(StripAnsi(dirLabel)), lipgloss.Right, labelStyle.Render(sortLabel)),
		)

		var listRows []string
		listRows = append(listRows, headerRow)
		listRows = append(listRows, lipgloss.NewStyle().Foreground(PrimaryColor).Render(strings.Repeat("-", boxWidth)))

		maxListHeight := availHeight - 2
		if maxListHeight < 1 {
			maxListHeight = 1
		}

		for i := 0; i < maxListHeight; i++ {
			idx := fp.Offset + i
			if idx < len(fp.Entries) {
				entry := fp.Entries[idx]
				prefix := "[ ]"
				if entry.Name == ".." {
					prefix = "   "
				} else if entry.Selected {
					prefix = "[x]"
				}

				dirPrefix := ""
				if entry.IsDir && entry.Name != ".." {
					dirPrefix = "📁 "
				} else if !entry.IsDir && entry.Name != ".." {
					dirPrefix = "📄 "
				}

				nameStr := fmt.Sprintf("%s %s%s", prefix, dirPrefix, entry.Name)
				sizeStr := ""
				if !entry.IsDir && entry.Name != ".." {
					sizeStr = ui.FormatBytes(entry.Size)
				}

				nameWidth := boxWidth - len(StripAnsi(sizeStr))
				if len(StripAnsi(nameStr)) > nameWidth {
					nameStr = Truncate(nameStr, nameWidth)
				}

				paddingCount := boxWidth - len(StripAnsi(nameStr)) - len(StripAnsi(sizeStr))
				if paddingCount < 1 {
					paddingCount = 1
				}

				fullLine := nameStr + strings.Repeat(" ", paddingCount) + sizeStr

				if idx == fp.Cursor {
					listRows = append(listRows, selectedStyle.Render(fullLine))
				} else {
					listRows = append(listRows, normalStyle.Render(fullLine))
				}
			} else {
				listRows = append(listRows, "")
			}
		}

		listContent := strings.Join(listRows, "\n")
		menuBox := lipgloss.NewStyle().Width(boxWidth).Render(listContent)

		return lipgloss.Place(
			w,
			availHeight,
			lipgloss.Center,
			lipgloss.Center,
			menuBox,
		)
	})
}
