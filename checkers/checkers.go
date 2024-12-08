package checkers

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/pkg/errors"
)

type Checker interface {
	Kind() string
	Check(ctx context.Context) error
}

var (
	registry      = map[string]func(in json.RawMessage) (Checker, error){}
	registryMutex = &sync.RWMutex{}
)

func NewCheckerByKind(kind string, spec json.RawMessage) (Checker, error) {
	registryMutex.RLock()
	defer registryMutex.RUnlock()

	c, ok := registry[kind]
	if !ok {
		return nil, errors.Errorf("checker with kind `%s` is not registered", kind)
	}

	return c(spec)
}

func Register(kind string, fn func(in json.RawMessage) (Checker, error)) error {
	registryMutex.Lock()
	defer registryMutex.Unlock()

	if _, ok := registry[kind]; ok {
		return errors.Errorf("checker with kind `%s` already registered", kind)
	}

	registry[kind] = fn

	return nil
}

func MustRegister(kind string, fn func(in json.RawMessage) (Checker, error)) {
	if err := Register(kind, fn); err != nil {
		panic(err)
	}
}
