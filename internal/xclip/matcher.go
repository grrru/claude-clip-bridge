package xclip

import "strings"

type Match struct {
	selection string
	output    bool
	target    string
}

func ParseArgs(args []string) Match {
	var match Match

	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "-o":
			match.output = true
		case "-selection", "-sel":
			if index+1 < len(args) {
				match.selection = args[index+1]
				index++
			}
		case "-t":
			if index+1 < len(args) {
				match.target = args[index+1]
				index++
			}
		}
	}

	return match
}

func (m Match) IsClipboardRead() bool {
	return strings.EqualFold(m.selection, "clipboard") && m.output && (m.IsImageRequest() || m.IsTargetsProbe())
}

func (m Match) IsTargetsProbe() bool {
	return strings.EqualFold(m.target, "TARGETS")
}

func (m Match) IsImageRequest() bool {
	return strings.HasPrefix(strings.ToLower(m.target), "image/")
}

func (m Match) IsPNGRequest() bool {
	return strings.EqualFold(m.target, "image/png")
}
