package objecttranslation

import "context"

type loaderKey struct {
	objectType string
	objectID   int
	field      string
	fallback   string
}

// Loader deduplicates translation work within one request.
type Loader struct {
	ctx     context.Context
	service *Service
	locale  string
	loaded  map[loaderKey]ResolvedValue
}

// NewLoader creates a request-scoped translation loader.
func NewLoader(ctx context.Context, service *Service, locale string) (*Loader, error) {
	normalizedLocale, err := NormalizeLocale(locale)
	if err != nil {
		return nil, err
	}
	return &Loader{
		ctx:     ctx,
		service: service,
		locale:  normalizedLocale,
		loaded:  make(map[loaderKey]ResolvedValue),
	}, nil
}

// Resolve returns values in input order and only sends unseen targets to the service.
func (l *Loader) Resolve(targets []Target) ([]ResolvedValue, error) {
	missing := make([]Target, 0, len(targets))
	pending := make(map[loaderKey]struct{})
	for _, target := range targets {
		key := loaderKey{target.ObjectType, target.ObjectID, target.Field, target.Fallback}
		if _, ok := l.loaded[key]; ok {
			continue
		}
		if _, ok := pending[key]; ok {
			continue
		}
		pending[key] = struct{}{}
		missing = append(missing, target)
	}
	if len(missing) > 0 {
		resolved, err := l.service.Resolve(l.ctx, l.locale, missing)
		if err != nil {
			return nil, err
		}
		for i, target := range missing {
			key := loaderKey{target.ObjectType, target.ObjectID, target.Field, target.Fallback}
			l.loaded[key] = resolved[i]
		}
	}

	values := make([]ResolvedValue, 0, len(targets))
	for _, target := range targets {
		key := loaderKey{target.ObjectType, target.ObjectID, target.Field, target.Fallback}
		values = append(values, l.loaded[key])
	}
	return values, nil
}
