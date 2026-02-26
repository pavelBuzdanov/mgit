package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

type menuResult struct {
	Kind  string // index|custom|cancel
	Index int
}

func (a *App) pickOptionInteractive(title string, items []string) (menuResult, error) {
	if len(items) == 0 {
		return menuResult{}, errors.New("no items to select")
	}
	if !a.stdinIsTTY() || !a.stdoutIsTTY() {
		return a.pickOptionLinePrompt(title, items)
	}

	stdinFile, ok := a.stdin.(*os.File)
	if !ok {
		return a.pickOptionLinePrompt(title, items)
	}

	raw, err := newRawTerminal(stdinFile)
	if err != nil {
		return a.pickOptionLinePrompt(title, items)
	}
	defer raw.restore()

	selected := 0
	numberBuf := ""
	lastLines := 0
	hideCursor(a.stdout)
	defer showCursor(a.stdout)

	render := func() {
		lines := renderMenuLines(title, items, selected, numberBuf)
		redrawLines(a.stdout, lines, &lastLines)
	}
	render()

	r := bufio.NewReader(a.stdin)
	for {
		b, err := r.ReadByte()
		if err != nil {
			return menuResult{}, err
		}
		switch b {
		case 3: // Ctrl+C
			return menuResult{}, errors.New("cancelled")
		case 13, 10: // Enter
			if strings.TrimSpace(numberBuf) != "" {
				n, convErr := strconv.Atoi(numberBuf)
				if convErr == nil {
					switch {
					case n >= 1 && n <= len(items):
						return menuResult{Kind: "index", Index: n - 1}, nil
					case n == len(items)+1:
						return menuResult{Kind: "custom"}, nil
					case n == len(items)+2:
						return menuResult{Kind: "cancel"}, nil
					}
				}
				numberBuf = ""
				render()
				continue
			}
			if selected < len(items) {
				return menuResult{Kind: "index", Index: selected}, nil
			}
			if selected == len(items) {
				return menuResult{Kind: "custom"}, nil
			}
			return menuResult{Kind: "cancel"}, nil
		case 27: // ESC / arrow sequence
			b2, _ := r.ReadByte()
			if b2 != '[' {
				numberBuf = ""
				render()
				continue
			}
			b3, _ := r.ReadByte()
			switch b3 {
			case 'A':
				if selected > 0 {
					selected--
				} else {
					selected = len(items) + 1
				}
				numberBuf = ""
				render()
			case 'B':
				if selected < len(items)+1 {
					selected++
				} else {
					selected = 0
				}
				numberBuf = ""
				render()
			default:
				numberBuf = ""
				render()
			}
		default:
			switch {
			case b >= '1' && b <= '9':
				numberBuf += string(b)
				render()
			case b == '0':
				if numberBuf != "" {
					numberBuf += "0"
					render()
				}
			case b == 'k' || b == 'K':
				if selected > 0 {
					selected--
				} else {
					selected = len(items) + 1
				}
				numberBuf = ""
				render()
			case b == 'j' || b == 'J':
				if selected < len(items)+1 {
					selected++
				} else {
					selected = 0
				}
				numberBuf = ""
				render()
			case b == 'c' || b == 'C':
				return menuResult{Kind: "custom"}, nil
			case b == 'q' || b == 'Q':
				return menuResult{Kind: "cancel"}, nil
			case b == 127 || b == 8: // backspace
				if numberBuf != "" {
					numberBuf = numberBuf[:len(numberBuf)-1]
					render()
				}
			}
		}
	}
}

func (a *App) pickOptionLinePrompt(title string, items []string) (menuResult, error) {
	fmt.Fprintln(a.stdout, title)
	for i, item := range items {
		fmt.Fprintf(a.stdout, "  %d) %s\n", i+1, item)
	}
	fmt.Fprintf(a.stdout, "  %d) Custom path\n", len(items)+1)
	fmt.Fprintf(a.stdout, "  %d) Cancel\n", len(items)+2)
	for {
		answer, err := a.promptLine("Choose option: ")
		if err != nil {
			return menuResult{}, err
		}
		n, err := strconv.Atoi(strings.TrimSpace(answer))
		if err != nil {
			fmt.Fprintln(a.stdout, "Invalid selection")
			continue
		}
		switch {
		case n >= 1 && n <= len(items):
			return menuResult{Kind: "index", Index: n - 1}, nil
		case n == len(items)+1:
			return menuResult{Kind: "custom"}, nil
		case n == len(items)+2:
			return menuResult{Kind: "cancel"}, nil
		default:
			fmt.Fprintln(a.stdout, "Invalid selection")
		}
	}
}

func renderMenuLines(title string, items []string, selected int, numberBuf string) []string {
	lines := []string{
		title,
		"Use ↑/↓ + Enter, or type a number. (c=custom, q=cancel)",
	}
	for i, item := range items {
		lines = append(lines, menuLine(i == selected, fmt.Sprintf("%d) %s", i+1, item)))
	}
	lines = append(lines, menuLine(len(items) == selected, fmt.Sprintf("%d) Custom path", len(items)+1)))
	lines = append(lines, menuLine(len(items)+1 == selected, fmt.Sprintf("%d) Cancel", len(items)+2)))
	if numberBuf != "" {
		lines = append(lines, "Number input: "+numberBuf)
	} else {
		lines = append(lines, "")
	}
	return lines
}

func menuLine(active bool, text string) string {
	if !active {
		return "  " + text
	}
	// Bright high-contrast highlight.
	return "\x1b[1;97;44m" + "  " + text + "\x1b[0m"
}

func redrawLines(w io.Writer, lines []string, lastCount *int) {
	if *lastCount > 0 {
		fmt.Fprintf(w, "\x1b[%dA", *lastCount)
	}
	for _, line := range lines {
		fmt.Fprintf(w, "\x1b[2K\r%s\n", line)
	}
	*lastCount = len(lines)
}

func hideCursor(w io.Writer) { fmt.Fprint(w, "\x1b[?25l") }
func showCursor(w io.Writer) { fmt.Fprint(w, "\x1b[?25h") }

type rawTerminal struct {
	stdin *os.File
	state string
}

func newRawTerminal(stdin *os.File) (*rawTerminal, error) {
	get := exec.Command("stty", "-g")
	get.Stdin = stdin
	out, err := get.Output()
	if err != nil {
		return nil, err
	}
	state := strings.TrimSpace(string(out))
	set := exec.Command("stty", "raw", "-echo")
	set.Stdin = stdin
	if err := set.Run(); err != nil {
		return nil, err
	}
	return &rawTerminal{stdin: stdin, state: state}, nil
}

func (r *rawTerminal) restore() {
	if r == nil || r.stdin == nil || r.state == "" {
		return
	}
	cmd := exec.Command("stty", r.state)
	cmd.Stdin = r.stdin
	_ = cmd.Run()
}

func (a *App) stdoutIsTTY() bool {
	f, ok := a.stdout.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}
