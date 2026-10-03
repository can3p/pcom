package translate

import (
	"fmt"
	"sort"
	"sync"

	"github.com/can3p/pcom/pkg/config"
)

// Factory builds a backend from the translation settings; it reads only its
// own nested group.
type Factory func(cfg config.Translation) (Backend, error)

var (
	regMu     sync.RWMutex
	factories = map[string]Factory{}
)

// Register makes a backend selectable as TRANSLATION_PROVIDER=name. A backend
// package calls it from init; the binary imports the package for its side
// effect. Registering a name twice panics.
func Register(name string, f Factory) {
	regMu.Lock()
	defer regMu.Unlock()

	if _, dup := factories[name]; dup {
		panic("translate: backend " + name + " registered twice")
	}

	factories[name] = f
}

// Registered lists the registered backend names, sorted.
func Registered() []string {
	regMu.RLock()
	defer regMu.RUnlock()

	names := make([]string, 0, len(factories))
	for n := range factories {
		names = append(names, n)
	}

	sort.Strings(names)

	return names
}

// New returns a Translator over the backend cfg.Provider names, or nil when
// the provider is empty: translation is off, and callers check for nil.
func New(cfg config.Translation, opts ...Option) (*Translator, error) {
	if cfg.Provider == "" {
		return nil, nil //nolint:nilnil // nil is "translation is off"
	}

	regMu.RLock()
	f, ok := factories[cfg.Provider]
	regMu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("translate: no backend %q is compiled in (have %v)", cfg.Provider, Registered())
	}

	b, err := f(cfg)
	if err != nil {
		return nil, fmt.Errorf("translate: %s: %w", cfg.Provider, err)
	}

	return NewTranslator(b, opts...), nil
}
