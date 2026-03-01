package printer

import (
	"regexp"
	"strings"
)

// Markdown-to-HTML converter. Zero external dependencies.
// Supports: headers, bold, italic, inline code, code blocks, lists,
// blockquotes, horizontal rules, links, and images.

type mdState int

const (
	stateNormal mdState = iota
	stateCodeBlock
	stateUnorderedList
	stateOrderedList
	stateBlockquote
)

var (
	reBold      = regexp.MustCompile(`\*\*(.+?)\*\*`)
	reItalic    = regexp.MustCompile(`\*(.+?)\*`)
	reCode      = regexp.MustCompile("`([^`]+)`")
	reLink      = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	reImage     = regexp.MustCompile(`!\[([^\]]*)\]\(([^)]+)\)`)
	reOLItem    = regexp.MustCompile(`^\d+\.\s+(.*)`)
	reHeader    = regexp.MustCompile(`^(#{1,6})\s+(.*)`)
)

// ConvertMarkdownToHTML converts Markdown text to a complete HTML document
// with print-friendly CSS styling.
func ConvertMarkdownToHTML(md string) string {
	lines := strings.Split(strings.ReplaceAll(md, "\r\n", "\n"), "\n")
	var out strings.Builder
	state := stateNormal

	out.WriteString(htmlDocHeader)

	for i := 0; i < len(lines); i++ {
		line := lines[i]

		// Handle code blocks
		if strings.HasPrefix(line, "```") {
			if state == stateCodeBlock {
				out.WriteString("</code></pre>\n")
				state = stateNormal
			} else {
				closeList(&out, state)
				state = stateCodeBlock
				out.WriteString("<pre><code>")
			}
			continue
		}
		if state == stateCodeBlock {
			out.WriteString(htmlEscape(line))
			out.WriteString("\n")
			continue
		}

		trimmed := strings.TrimSpace(line)

		// Blank line closes lists/blockquotes
		if trimmed == "" {
			closeList(&out, state)
			state = stateNormal
			continue
		}

		// Horizontal rule
		if trimmed == "---" || trimmed == "***" || trimmed == "___" {
			closeList(&out, state)
			state = stateNormal
			out.WriteString("<hr>\n")
			continue
		}

		// Header
		if m := reHeader.FindStringSubmatch(trimmed); m != nil {
			closeList(&out, state)
			state = stateNormal
			level := len(m[1])
			out.WriteString("<h")
			out.WriteByte(byte('0' + level))
			out.WriteString(">")
			out.WriteString(processInline(m[2]))
			out.WriteString("</h")
			out.WriteByte(byte('0' + level))
			out.WriteString(">\n")
			continue
		}

		// Blockquote
		if strings.HasPrefix(trimmed, "> ") {
			if state != stateBlockquote {
				closeList(&out, state)
				state = stateBlockquote
				out.WriteString("<blockquote>\n")
			}
			content := strings.TrimPrefix(trimmed, "> ")
			out.WriteString("<p>")
			out.WriteString(processInline(content))
			out.WriteString("</p>\n")
			continue
		}

		// Unordered list
		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
			if state != stateUnorderedList {
				closeList(&out, state)
				state = stateUnorderedList
				out.WriteString("<ul>\n")
			}
			content := trimmed[2:]
			out.WriteString("<li>")
			out.WriteString(processInline(content))
			out.WriteString("</li>\n")
			continue
		}

		// Ordered list
		if m := reOLItem.FindStringSubmatch(trimmed); m != nil {
			if state != stateOrderedList {
				closeList(&out, state)
				state = stateOrderedList
				out.WriteString("<ol>\n")
			}
			out.WriteString("<li>")
			out.WriteString(processInline(m[1]))
			out.WriteString("</li>\n")
			continue
		}

		// Regular paragraph
		if state != stateNormal {
			closeList(&out, state)
			state = stateNormal
		}
		out.WriteString("<p>")
		out.WriteString(processInline(trimmed))
		out.WriteString("</p>\n")
	}

	// Close any open state
	if state == stateCodeBlock {
		out.WriteString("</code></pre>\n")
	} else {
		closeList(&out, state)
	}

	out.WriteString("</body>\n</html>\n")
	return out.String()
}

func closeList(out *strings.Builder, state mdState) {
	switch state {
	case stateUnorderedList:
		out.WriteString("</ul>\n")
	case stateOrderedList:
		out.WriteString("</ol>\n")
	case stateBlockquote:
		out.WriteString("</blockquote>\n")
	}
}

func htmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}

func processInline(s string) string {
	s = htmlEscape(s)

	// Images (must be before links since images contain link syntax)
	s = reImage.ReplaceAllString(s, `<img src="$2" alt="$1" style="max-width:100%">`)

	// Links
	s = reLink.ReplaceAllString(s, `<a href="$2">$1</a>`)

	// Inline code
	s = reCode.ReplaceAllString(s, `<code>$1</code>`)

	// Bold
	s = reBold.ReplaceAllString(s, `<strong>$1</strong>`)

	// Italic
	s = reItalic.ReplaceAllString(s, `<em>$1</em>`)

	return s
}

const htmlDocHeader = `<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<style>
body {
    font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif;
    line-height: 1.6;
    max-width: 8in;
    margin: 0 auto;
    padding: 0.5in;
    color: #222;
}
h1, h2, h3, h4, h5, h6 {
    margin-top: 1.2em;
    margin-bottom: 0.4em;
    line-height: 1.2;
}
h1 { font-size: 2em; border-bottom: 2px solid #ccc; padding-bottom: 0.2em; }
h2 { font-size: 1.5em; border-bottom: 1px solid #ddd; padding-bottom: 0.2em; }
h3 { font-size: 1.2em; }
p { margin: 0.6em 0; }
code {
    font-family: 'Cascadia Code', 'Consolas', 'Courier New', monospace;
    background: #f4f4f4;
    padding: 0.15em 0.3em;
    border-radius: 3px;
    font-size: 0.9em;
}
pre {
    background: #f4f4f4;
    padding: 1em;
    border-radius: 4px;
    overflow-x: auto;
    border: 1px solid #ddd;
}
pre code {
    background: none;
    padding: 0;
}
blockquote {
    border-left: 4px solid #ddd;
    margin: 0.8em 0;
    padding: 0.4em 1em;
    color: #555;
}
ul, ol { margin: 0.6em 0; padding-left: 2em; }
li { margin: 0.2em 0; }
hr { border: none; border-top: 1px solid #ccc; margin: 1.5em 0; }
a { color: #0066cc; }
img { max-width: 100%; height: auto; }
@media print {
    body { margin: 0; padding: 0; max-width: none; }
    pre { white-space: pre-wrap; word-wrap: break-word; }
    a { color: #000; text-decoration: underline; }
    a[href]:after { content: " (" attr(href) ")"; font-size: 0.8em; }
}
</style>
</head>
<body>
`
