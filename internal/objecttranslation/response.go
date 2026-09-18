package objecttranslation

import (
	"context"
	"fmt"
	"net/http"
	"reflect"

	"golang.org/x/text/language"
)

// RequestLocale returns the highest-priority valid locale from Accept-Language.
func RequestLocale(r *http.Request) string {
	tags, _, err := language.ParseAcceptLanguage(r.Header.Get("Accept-Language"))
	if err == nil && len(tags) > 0 && tags[0] != language.Und {
		return tags[0].String()
	}
	return "en"
}

var responseFields = []struct {
	canonical string
	display   string
	field     string
}{
	{"Name", "DisplayName", FieldName},
	{"Description", "DisplayDescription", FieldDescription},
	{"ForwardLabel", "DisplayForwardLabel", FieldForwardLabel},
	{"ReverseLabel", "DisplayReverseLabel", FieldReverseLabel},
}

// LocalizeResponse populates display fields for names, descriptions, and link
// direction labels while retaining canonical values. Value may be a struct
// pointer or a slice of structs, pointers, or interfaces containing either.
func (s *Service) LocalizeResponse(ctx context.Context, locale, objectType string, value any) error {
	objects, err := responseObjects(value)
	if err != nil {
		return err
	}
	if len(objects) == 0 {
		return nil
	}

	targets := make([]Target, 0, len(objects)*len(responseFields))
	for _, object := range objects {
		for _, field := range responseFields {
			if !object.FieldByName(field.display).IsValid() {
				continue
			}
			targets = append(targets, Target{
				ObjectType: objectType,
				ObjectID:   int(object.FieldByName("ID").Int()),
				Field:      field.field,
				Fallback:   object.FieldByName(field.canonical).String(),
			})
		}
	}

	loader, err := NewLoader(ctx, s, locale)
	if err != nil {
		return err
	}
	resolved, err := loader.Resolve(targets)
	if err != nil {
		return err
	}
	resolvedIndex := 0
	for _, object := range objects {
		for _, field := range responseFields {
			display := object.FieldByName(field.display)
			if !display.IsValid() {
				continue
			}
			display.SetString(resolved[resolvedIndex].Value)
			resolvedIndex++
		}
	}
	return nil
}

func responseObjects(value any) ([]reflect.Value, error) {
	root := reflect.ValueOf(value)
	if !root.IsValid() {
		return nil, nil
	}
	for root.Kind() == reflect.Pointer || root.Kind() == reflect.Interface {
		if root.IsNil() {
			return nil, nil
		}
		root = root.Elem()
	}

	values := []reflect.Value{root}
	if root.Kind() == reflect.Slice {
		values = make([]reflect.Value, root.Len())
		for i := range root.Len() {
			values[i] = root.Index(i)
		}
	}

	objects := make([]reflect.Value, 0, len(values))
	for _, candidate := range values {
		for candidate.Kind() == reflect.Pointer || candidate.Kind() == reflect.Interface {
			if candidate.IsNil() {
				break
			}
			candidate = candidate.Elem()
		}
		if candidate.Kind() != reflect.Struct {
			return nil, fmt.Errorf("localize response: expected struct, got %s", candidate.Kind())
		}
		id := candidate.FieldByName("ID")
		name := candidate.FieldByName("Name")
		displayName := candidate.FieldByName("DisplayName")
		if !id.IsValid() || id.Kind() != reflect.Int || !name.IsValid() || name.Kind() != reflect.String ||
			!displayName.IsValid() || displayName.Kind() != reflect.String || !displayName.CanSet() {
			return nil, fmt.Errorf("localize response: value must expose settable ID, Name, and DisplayName fields")
		}
		for _, field := range responseFields[1:] {
			if display := candidate.FieldByName(field.display); display.IsValid() {
				canonical := candidate.FieldByName(field.canonical)
				if display.Kind() != reflect.String || !display.CanSet() ||
					!canonical.IsValid() || canonical.Kind() != reflect.String {
					return nil, fmt.Errorf("localize response: %s requires a string %s field", field.display, field.canonical)
				}
			}
		}
		objects = append(objects, candidate)
	}
	return objects, nil
}
