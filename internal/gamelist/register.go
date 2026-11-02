package gamelist

import (
	"encoding/xml"
	"errors"
	"os"
	"regexp"
	"strings"
)

// Entry is the metadata PlayTime manages for its own launcher entry.
type Entry struct {
	Path, Name, Desc, Image, Thumbnail string
}

// Register adds or updates one <game> entry in a gamelist.xml. It edits the
// file as text and only touches the matching entry, so fields it does not know
// about (playcount, gametime, scrap data, other games) are left byte-for-byte
// intact. Nothing is written when the entry is already correct, since every
// write makes ES reload the list.
func Register(glPath string, e Entry) error {
	data, err := os.ReadFile(glPath)
	if errors.Is(err, os.ErrNotExist) {
		data = []byte("<?xml version=\"1.0\"?>\n<gameList>\n</gameList>\n")
	} else if err != nil {
		return err
	}

	out, changed, err := upsert(string(data), e)
	if err != nil || !changed {
		return err
	}
	return writeAtomic(glPath, []byte(out))
}

func upsert(text string, e Entry) (string, bool, error) {
	fields := [][2]string{
		{"name", e.Name},
		{"desc", e.Desc},
		{"image", e.Image},
		{"thumbnail", e.Thumbnail},
	}

	start, end, ok := findGame(text, e.Path)
	if !ok {
		closeIdx := strings.LastIndex(text, "</gameList>")
		if closeIdx < 0 {
			return "", false, errors.New("gamelist.xml has no </gameList>, refusing to edit")
		}
		var b strings.Builder
		b.WriteString("\t<game>\n")
		b.WriteString("\t\t<path>" + escape(e.Path) + "</path>\n")
		for _, f := range fields {
			b.WriteString("\t\t<" + f[0] + ">" + escape(f[1]) + "</" + f[0] + ">\n")
		}
		b.WriteString("\t</game>\n")
		return text[:closeIdx] + b.String() + text[closeIdx:], true, nil
	}

	block := text[start:end]
	orig := block
	for _, f := range fields {
		block = setField(block, f[0], f[1])
	}
	if block == orig {
		return text, false, nil
	}
	return text[:start] + block + text[end:], true, nil
}

var (
	gameRe = regexp.MustCompile(`(?s)<game(?:\s[^>]*)?>.*?</game>`)
	pathRe = regexp.MustCompile(`(?s)<path>(.*?)</path>`)
)

// findGame returns the byte range of the <game>…</game> block whose <path> is path.
func findGame(text, path string) (int, int, bool) {
	for _, loc := range gameRe.FindAllStringIndex(text, -1) {
		m := pathRe.FindStringSubmatch(text[loc[0]:loc[1]])
		if m != nil && strings.TrimSpace(unescape(m[1])) == path {
			return loc[0], loc[1], true
		}
	}
	return 0, 0, false
}

// setField replaces <tag>…</tag> (or <tag/>) inside block, or inserts it before </game>.
func setField(block, tag, value string) string {
	re := regexp.MustCompile(`(?s)<` + tag + `\s*/>|<` + tag + `>(.*?)</` + tag + `>`)
	elem := "<" + tag + ">" + escape(value) + "</" + tag + ">"
	if m := re.FindStringSubmatchIndex(block); m != nil {
		if m[2] >= 0 && unescape(block[m[2]:m[3]]) == value {
			return block
		}
		return block[:m[0]] + elem + block[m[1]:]
	}
	i := strings.LastIndex(block, "</game>")
	lineStart := strings.LastIndex(block[:i], "\n") + 1
	closeIndent := block[lineStart:i]
	if lineStart == 0 || strings.TrimSpace(closeIndent) != "" {
		return block[:i] + elem + block[i:] // single-line block, no indent to match
	}
	// New line just above </game>, one level deeper than it.
	return block[:lineStart] + closeIndent + "\t" + elem + "\n" + block[lineStart:]
}

// escape matches what ES writes: only &, < and > are escaped in text nodes.
var escaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

func escape(s string) string { return escaper.Replace(s) }

func unescape(s string) string {
	var v struct {
		Text string `xml:",chardata"`
	}
	if xml.Unmarshal([]byte("<v>"+s+"</v>"), &v) != nil {
		return s
	}
	return v.Text
}

func writeAtomic(path string, data []byte) error {
	tmp := path + ".playtime.tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
