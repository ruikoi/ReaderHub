package bundle

import (
	"bytes"
	"fmt"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
	xhtml "golang.org/x/net/html"
	"readerhub/internal/document"
	"sort"
	"strings"
)

// PortableMarkdown changes actual image nodes, leaving prose, code examples and
// the source file untouched. Reference-style images become self-contained inline images.
func PortableMarkdown(body []byte, assets []document.Asset) ([]byte, error) {
	resources := map[string]string{}
	for _, a := range assets {
		fragment := ""
		if i := strings.IndexByte(a.Original, '#'); i >= 0 {
			fragment = a.Original[i:]
		}
		resources[a.Original] = "./assets/" + a.ID + fragment
	}
	type edit struct {
		start, end int
		value      string
	}
	edits := []edit{}
	root := goldmark.DefaultParser().Parse(text.NewReader(body))
	err := ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch node := n.(type) {
		case *ast.Image:
			dest := normalizeReference(string(node.Destination))
			target, ok := resources[dest]
			if !ok {
				return ast.WalkContinue, nil
			}
			start := node.Pos()
			if start > 0 && body[start-1] == '!' {
				start--
			}
			if start < 0 || start+1 >= len(body) || string(body[start:start+2]) != "![" {
				return ast.WalkStop, fmt.Errorf("cannot locate image in source")
			}
			altEnd := balancedEnd(body, start+1, '[', ']')
			if altEnd < 0 {
				return ast.WalkStop, fmt.Errorf("invalid image source position")
			}
			end := altEnd + 1
			if node.Reference == nil {
				if end >= len(body) || body[end] != '(' {
					return ast.WalkStop, fmt.Errorf("cannot locate image destination")
				}
				end = balancedEnd(body, end, '(', ')') + 1
			} else if end < len(body) && body[end] == '[' {
				end = balancedEnd(body, end, '[', ']') + 1
			}
			if end <= altEnd {
				return ast.WalkStop, fmt.Errorf("cannot locate image ending")
			}
			title := ""
			if len(node.Title) > 0 {
				title = " \"" + strings.ReplaceAll(string(node.Title), "\"", "\\\"") + "\""
			}
			edits = append(edits, edit{start, end, string(body[start:altEnd+1]) + "(" + target + title + ")"})
		case *ast.RawHTML:
			if node.Segments.Len() > 0 {
				start := node.Segments.At(0).Start
				end := node.Segments.At(node.Segments.Len() - 1).Stop
				edits = append(edits, edit{start, end, rewriteHTML(body[start:end], resources)})
			}
		case *ast.HTMLBlock:
			if node.Lines().Len() > 0 {
				start := node.Lines().At(0).Start
				end := node.Lines().At(node.Lines().Len() - 1).Stop
				if node.HasClosure() {
					end = node.ClosureLine.Stop
				}
				edits = append(edits, edit{start, end, rewriteHTML(body[start:end], resources)})
			}
		}
		return ast.WalkContinue, nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(edits, func(i, j int) bool { return edits[i].start > edits[j].start })
	result := append([]byte{}, body...)
	last := len(body)
	for _, e := range edits {
		if e.start < 0 || e.end > last || e.end < e.start {
			return nil, fmt.Errorf("overlapping image source ranges")
		}
		result = append(append(append([]byte{}, result[:e.start]...), []byte(e.value)...), result[e.end:]...)
		last = e.start
	}
	return result, nil
}

func balancedEnd(body []byte, start int, open, close byte) int {
	depth := 0
	quoted := byte(0)
	angle := false
	for i := start; i < len(body); i++ {
		c := body[i]
		if c == '\\' {
			i++
			continue
		}
		if open == '(' && c == '<' && quoted == 0 {
			angle = true
			continue
		}
		if angle {
			if c == '>' {
				angle = false
			}
			continue
		}
		if open == '(' && (c == '\'' || c == '"') && (quoted != 0 || i > start && (body[i-1] == ' ' || body[i-1] == '\n' || body[i-1] == '\t')) {
			if quoted == 0 {
				quoted = c
			} else if quoted == c {
				quoted = 0
			}
			continue
		}
		if quoted != 0 {
			continue
		}
		if c == open {
			depth++
		}
		if c == close {
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}
func rewriteHTML(raw []byte, resources map[string]string) string {
	var result bytes.Buffer
	t := xhtml.NewTokenizer(bytes.NewReader(raw))
	for {
		kind := t.Next()
		if kind == xhtml.ErrorToken {
			result.Write(t.Raw())
			break
		}
		original := append([]byte{}, t.Raw()...)
		if kind == xhtml.StartTagToken || kind == xhtml.SelfClosingTagToken {
			token := t.Token()
			if token.Data == "img" {
				for i, a := range token.Attr {
					if a.Key == "src" {
						if target, ok := resources[a.Val]; ok {
							token.Attr[i].Val = target
						}
					}
				}
				result.WriteString(token.String())
				continue
			}
		}
		result.Write(original)
	}
	return result.String()
}
