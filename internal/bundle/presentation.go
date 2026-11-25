package bundle

import (
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
	"strings"
)

// Presentation extracts existing text only; it never generates or analyzes content.
func Presentation(body []byte) (title, preview string) {
	if strings.HasPrefix(string(body), "---\n") {
		if end := strings.Index(string(body[4:]), "\n---\n"); end >= 0 {
			body = body[4+end+5:]
		}
	}
	root := goldmark.DefaultParser().Parse(text.NewReader(body))
	paragraphs := []string{}
	ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch node := n.(type) {
		case *ast.Heading:
			if title == "" && node.Level == 1 {
				title = strings.Join(strings.Fields(string(node.Text(body))), " ")
			}
		case *ast.Paragraph:
			if node.FirstChild() != nil && node.FirstChild().Kind() == ast.KindImage && node.FirstChild().NextSibling() == nil {
				return ast.WalkSkipChildren, nil
			}
			value := strings.Join(strings.Fields(string(node.Text(body))), " ")
			if value != "" {
				paragraphs = append(paragraphs, value)
			}
		}
		return ast.WalkContinue, nil
	})
	return title, strings.Join(paragraphs, " ")
}
