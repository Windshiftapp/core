package emailutil

import (
	"reflect"

	"windshift/internal/i18n"
)

// EnrichWithSubject returns a value where the rendered Subject is accessible
// alongside the original fields, so HTML/text body templates can reference
// `{{.Subject}}` (e.g. for the <title> tag) without each call site having to
// remember to include it in the data struct. Uses the default locale.
func EnrichWithSubject(data any, subject string) any {
	return EnrichWithSubjectLocale(data, subject, i18n.Default)
}

// EnrichWithSubjectLocale is EnrichWithSubject and also exposes `{{.Locale}}`
// and `{{.Dir}}` so the shared shell can set lang/dir on <html>.
//
// For struct (and pointer-to-struct) inputs, exported fields are copied into
// a map[string]any preserving their original types — important so
// numeric fields keep `int` (not `float64`) for `eq` comparisons in
// templates. For non-struct inputs, the original value is returned under
// `Data` alongside `Subject` and templates can address it as `{{.Data.X}}`.
func EnrichWithSubjectLocale(data any, subject, locale string) any {
	enriched := enrich(data, subject)
	if m, ok := enriched.(map[string]any); ok {
		m["Locale"] = i18n.Normalize(locale)
		m["Dir"] = "ltr"
		if i18n.IsRTL(locale) {
			m["Dir"] = "rtl"
		}
	}
	return enriched
}

func enrich(data any, subject string) any {
	v := reflect.ValueOf(data)
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return map[string]any{"Subject": subject}
		}
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		return map[string]any{"Data": data, "Subject": subject}
	}

	t := v.Type()
	m := make(map[string]any, v.NumField()+1)
	for i := 0; i < v.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		m[f.Name] = v.Field(i).Interface()
	}
	m["Subject"] = subject
	return m
}
