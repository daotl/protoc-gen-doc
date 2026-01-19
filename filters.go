package gendoc

import (
	"fmt"
	"html/template"
	"regexp"
	"strings"
)

var (
	paraPattern         = regexp.MustCompile(`(\n|\r|\r\n)\s*`)
	spacePattern        = regexp.MustCompile("( )+")
	multiNewlinePattern = regexp.MustCompile(`(\r\n|\r|\n){2,}`)
	specialCharsPattern = regexp.MustCompile(`[^a-zA-Z0-9_-]`)
)

// PFilter splits the content by new lines and wraps each one in a <p> tag.
func PFilter(content string) template.HTML {
	paragraphs := paraPattern.Split(content, -1)
	return template.HTML(fmt.Sprintf("<p>%s</p>", strings.Join(paragraphs, "</p><p>")))
}

// ParaFilter splits the content by new lines and wraps each one in a <para> tag.
func ParaFilter(content string) string {
	paragraphs := paraPattern.Split(content, -1)
	return fmt.Sprintf("<para>%s</para>", strings.Join(paragraphs, "</para><para>"))
}

// NoBrFilter removes single CR and LF from content, replacing them with <br> for proper
// rendering in markdown and HTML tables.
func NoBrFilter(content string) template.HTML {
	normalized := strings.Replace(content, "\r\n", "\n", -1)
	paragraphs := multiNewlinePattern.Split(normalized, -1)
	for i, p := range paragraphs {
		// First normalize multiple spaces to single space
		p = spacePattern.ReplaceAllString(p, " ")
		// Then replace newlines with <br>
		withoutCR := strings.Replace(p, "\r", "<br>", -1)
		withoutLF := strings.Replace(withoutCR, "\n", "<br>", -1)
		// Trim any extra spaces around <br> tags
		withoutLF = strings.Replace(withoutLF, " <br>", "<br>", -1)
		withoutLF = strings.Replace(withoutLF, "<br> ", "<br>", -1)
		paragraphs[i] = withoutLF
	}
	// Join paragraphs with <br><br> instead of \n\n for proper table rendering
	return template.HTML(strings.Join(paragraphs, "<br><br>"))
}

// AnchorFilter replaces all special characters with URL friendly dashes
func AnchorFilter(str string) string {
	return specialCharsPattern.ReplaceAllString(strings.ReplaceAll(str, "/", "_"), "-")
}
