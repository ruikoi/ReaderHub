// Package research reserves an integration boundary; the first version performs no AI work.
package research

import (
	"context"
	"errors"
)

var ErrDisabled = errors.New("AI research is not implemented")

type Request struct {
	ArticleID string
	NoteIDs   []string
}
type Result struct{ DocumentID string }
type Service interface {
	Organize(context.Context, Request) (Result, error)
}
type Disabled struct{}

func (Disabled) Organize(context.Context, Request) (Result, error) { return Result{}, ErrDisabled }
