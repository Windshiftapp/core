package emailutil

import "reflect"

// EnrichWithSubject returns a value where the rendered Subject is accessible
// alongside the original fields, so HTML/text body templates can reference
// `{{.Subject}}` (e.g. for the <title> tag) without each call site having to
// remember to include it in the data struct.
//
// For struct (and pointer-to-struct) inputs, exported fields are copied into
// a map[string]interface{} preserving their original types — important so
// numeric fields keep `int` (not `float64`) for `eq` comparisons in
// templates. For non-struct inputs, the original value is returned under
// `Data` alongside `Subject` and templates can address it as `{{.Data.X}}`.
func EnrichWithSubject(data interface{}, subject string) interface{} {
	v := reflect.ValueOf(data)
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return map[string]interface{}{"Subject": subject}
		}
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		return map[string]interface{}{"Data": data, "Subject": subject}
	}

	t := v.Type()
	m := make(map[string]interface{}, v.NumField()+1)
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
