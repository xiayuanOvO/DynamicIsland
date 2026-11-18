package lyrics

import (
	"slices"
	"sort"
	"strconv"
	"strings"
)

type Line struct {
	At   float64
	Text string
}

type Lines []Line

func Parse(text string) Lines {
	var lines Lines
	var offset float64
	hasText := false
	for raw := range strings.SplitSeq(strings.TrimPrefix(text, "\ufeff"), "\n") {
		raw = strings.TrimSpace(raw)
		if value, ok := strings.CutPrefix(raw, "[offset:"); ok {
			if ms, err := strconv.ParseInt(strings.TrimSuffix(value, "]"), 10, 32); err == nil {
				offset = float64(ms) / 1000
			}
			continue
		}

		start := len(lines)
		for strings.HasPrefix(raw, "[") {
			tag, rest, ok := strings.Cut(raw[1:], "]")
			minutes, seconds, timed := strings.Cut(tag, ":")
			if !ok || !timed {
				break
			}
			min, minErr := strconv.ParseUint(minutes, 10, 32)
			sec, secErr := strconv.ParseFloat(strings.Replace(seconds, ":", ".", 1), 64)
			if minErr != nil || secErr != nil || !(sec >= 0 && sec < 60) {
				break
			}
			lines = append(lines, Line{At: float64(min)*60 + sec})
			raw = rest
		}
		if start < len(lines) {
			text := strings.TrimSpace(raw)
			hasText = hasText || text != ""
			for i := start; i < len(lines); i++ {
				lines[i].Text = text
			}
		}
	}
	if !hasText {
		return nil
	}
	for i := range lines {
		lines[i].At = max(0, lines[i].At+offset)
	}
	slices.SortStableFunc(lines, func(a, b Line) int {
		if a.At < b.At {
			return -1
		}
		if a.At > b.At {
			return 1
		}
		return 0
	})
	return lines
}

func (l Lines) Current(position float64) (text string, next float64) {
	rows, next := l.Surrounding(position)
	return rows[1], next
}

func (l Lines) Surrounding(position float64) (rows [3]string, next float64) {
	i := sort.Search(len(l), func(i int) bool { return l[i].At > position })
	if i > 0 {
		rows[1] = l[i-1].Text
		for prev := i - 2; prev >= 0; prev-- {
			if rows[0] = l[prev].Text; rows[0] != "" {
				break
			}
		}
	}
	if i < len(l) {
		next = l[i].At
		for _, line := range l[i:] {
			if rows[2] = line.Text; rows[2] != "" {
				break
			}
		}
	}
	return rows, next
}
