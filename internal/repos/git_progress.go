// ALR - Any Linux Repository
// Copyright (C) 2026 The ALR Authors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package repos

import (
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/leonelquinteros/gotext"
)

var (
	gitPercentProgress = regexp.MustCompile(`^([A-Za-z ]+):\s*(\d+)%\s*\((\d+)/(\d+)\)`)
	gitObjectCount     = regexp.MustCompile(`^([A-Za-z ]+):\s*([\d,]+)`)
	gitSpinnerFrames   = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
)

type gitProgressWriter struct {
	out     io.Writer
	pending strings.Builder
	active  bool
	closed  bool
	frame   int
}

func newGitProgressWriter(out io.Writer) io.WriteCloser {
	return &gitProgressWriter{out: out}
}

func (w *gitProgressWriter) Write(p []byte) (int, error) {
	if w.closed {
		return 0, io.ErrClosedPipe
	}

	for _, b := range p {
		switch b {
		case '\r', '\n':
			if err := w.render(w.pending.String()); err != nil {
				return 0, err
			}
			w.pending.Reset()
		default:
			w.pending.WriteByte(b)
		}
	}

	return len(p), nil
}

func (w *gitProgressWriter) Close() error {
	if w.closed {
		return nil
	}
	w.closed = true

	if w.pending.Len() > 0 {
		if err := w.render(w.pending.String()); err != nil {
			return err
		}
	}
	if w.active {
		_, err := fmt.Fprintln(w.out)
		return err
	}
	return nil
}

func (w *gitProgressWriter) render(message string) error {
	message = strings.TrimSpace(message)
	message = strings.TrimPrefix(message, "remote: ")
	if matches := gitPercentProgress.FindStringSubmatch(message); matches != nil {
		stage := localizedGitStage(matches[1])
		if stage == "" {
			return nil
		}
		percent, err := strconv.Atoi(matches[2])
		if err != nil {
			return nil
		}
		return w.writeLine(fmt.Sprintf("%s %s %3d%% (%s/%s)",
			stage, gitProgressBar(percent), percent, matches[3], matches[4]))
	}

	if matches := gitObjectCount.FindStringSubmatch(message); matches != nil {
		stage := localizedGitStage(matches[1])
		if stage == "" {
			return nil
		}
		frame := gitSpinnerFrames[w.frame%len(gitSpinnerFrames)]
		w.frame++
		return w.writeLine(fmt.Sprintf("%s %s: %s", frame, stage, matches[2]))
	}

	return nil
}

func (w *gitProgressWriter) writeLine(line string) error {
	prefix := "\r\033[K"
	if !w.active {
		prefix = ""
	}
	_, err := fmt.Fprint(w.out, prefix+line)
	w.active = true
	return err
}

func localizedGitStage(stage string) string {
	switch stage {
	case "Enumerating objects":
		return gotext.Get("Enumerating objects")
	case "Counting objects":
		return gotext.Get("Counting objects")
	case "Compressing objects":
		return gotext.Get("Compressing objects")
	case "Receiving objects":
		return gotext.Get("Receiving objects")
	case "Resolving deltas":
		return gotext.Get("Resolving deltas")
	default:
		return ""
	}
}

func gitProgressBar(percent int) string {
	percent = max(0, min(100, percent))
	const width = 40
	filled := percent * width / 100

	gradientColor := func(pos float64) string {
		var r, g, b int
		if pos < 0.5 {
			t := pos * 2
			r = int(239 + t*(234-239))
			g = int(68 + t*(179-68))
			b = int(68 + t*(8-68))
		} else {
			t := (pos - 0.5) * 2
			r = int(234 + t*(59-234))
			g = int(179 + t*(130-179))
			b = int(8 + t*(246-8))
		}
		return fmt.Sprintf("#%02x%02x%02x", r, g, b)
	}

	var bar strings.Builder
	for i := 0; i < width; i++ {
		if i < filled {
			pos := float64(i) / float64(width-1)
			bar.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(gradientColor(pos))).Render("●"))
		} else {
			bar.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("○"))
		}
	}
	return bar.String()
}
