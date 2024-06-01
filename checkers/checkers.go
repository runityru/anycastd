package checkers

import (
	"context"
	"encoding/json"

	"github.com/pkg/errors"
)

type Checker interface {
	Check(ctx context.Context) error
}

var registry = map[string]func(in json.RawMessage) (Checker, error){}

func NewCheckerByKind(kind string, spec json.RawMessage) (Checker, error) {
	c, ok := registry[kind]
	if !ok {
		return nil, errors.Errorf("checker with kind `%s` is not registered", kind)
	}

	return c(spec)
}

func Register(kind string, fn func(in json.RawMessage) (Checker, error)) error {
	if _, ok := registry[kind]; ok {
		return errors.Errorf("checker with kind `%s` already registered", kind)
	}

	registry[kind] = fn

	return nil
}
