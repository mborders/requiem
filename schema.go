package requiem

import (
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"
)

var timeType = reflect.TypeOf(time.Time{})

// OpenAPISchemaProvider lets a type declare its own OpenAPI schema fragment.
// requiem honors it before falling back to reflection, so types whose JSON
// shape can't be inferred from their Go declaration (custom MarshalJSON,
// JSON-encoded string aliases, etc.) can describe themselves accurately.
type OpenAPISchemaProvider interface {
	OpenAPISchema() map[string]interface{}
}

var openAPISchemaProviderType = reflect.TypeOf((*OpenAPISchemaProvider)(nil)).Elem()

func (db *docBuilder) schemaFor(t reflect.Type) map[string]interface{} {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	if schema, ok := customSchemaFor(t); ok {
		return schema
	}

	if t == timeType {
		return map[string]interface{}{"type": "string", "format": "date-time"}
	}

	switch t.Kind() {
	case reflect.String:
		return map[string]interface{}{"type": "string"}
	case reflect.Bool:
		return map[string]interface{}{"type": "boolean"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return map[string]interface{}{"type": "integer"}
	case reflect.Float32, reflect.Float64:
		return map[string]interface{}{"type": "number"}
	case reflect.Slice, reflect.Array:
		if t.Elem().Kind() == reflect.Uint8 {
			return map[string]interface{}{"type": "string", "format": "byte"}
		}
		return map[string]interface{}{
			"type":  "array",
			"items": db.schemaFor(t.Elem()),
		}
	case reflect.Map:
		return map[string]interface{}{
			"type":                 "object",
			"additionalProperties": db.schemaFor(t.Elem()),
		}
	case reflect.Struct:
		return db.structRef(t)
	case reflect.Interface:
		return map[string]interface{}{}
	}

	return map[string]interface{}{}
}

// customSchemaFor returns a schema produced by OpenAPISchemaProvider when the
// type (or its pointer) implements it. Skips reflect.Interface to avoid the
// special case of zero-value interface instantiation.
func customSchemaFor(t reflect.Type) (map[string]interface{}, bool) {
	if t.Kind() == reflect.Interface {
		return nil, false
	}
	if t.Implements(openAPISchemaProviderType) {
		return reflect.New(t).Elem().Interface().(OpenAPISchemaProvider).OpenAPISchema(), true
	}
	if reflect.PointerTo(t).Implements(openAPISchemaProviderType) {
		return reflect.New(t).Interface().(OpenAPISchemaProvider).OpenAPISchema(), true
	}
	return nil, false
}

func (db *docBuilder) structRef(t reflect.Type) map[string]interface{} {
	name := t.Name()
	if name == "" {
		return db.buildStructSchema(t)
	}

	if _, ok := db.schemas[name]; !ok {
		// reserve the slot first so recursive references return a $ref instead of recursing forever
		db.schemas[name] = map[string]interface{}{}
		db.schemas[name] = db.buildStructSchema(t)
	}
	return map[string]interface{}{"$ref": "#/components/schemas/" + name}
}

func (db *docBuilder) buildStructSchema(t reflect.Type) map[string]interface{} {
	properties := map[string]interface{}{}
	required := []string{}

	var visit func(t reflect.Type)
	visit = func(t reflect.Type) {
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if !f.IsExported() {
				continue
			}

			jsonTag := f.Tag.Get("json")
			if jsonTag == "-" {
				continue
			}

			// Anonymous embedded struct with no explicit json tag: Go's
			// encoding/json promotes its exported fields to the parent. Mirror
			// that in the schema so the generated client sees a flat object.
			if f.Anonymous && jsonTag == "" {
				ft := f.Type
				for ft.Kind() == reflect.Ptr {
					ft = ft.Elem()
				}
				if ft.Kind() == reflect.Struct {
					visit(ft)
					continue
				}
			}

			name := f.Name
			omitEmpty := false
			if jsonTag != "" {
				parts := strings.Split(jsonTag, ",")
				if parts[0] != "" {
					name = parts[0]
				}
				for _, p := range parts[1:] {
					if p == "omitempty" {
						omitEmpty = true
					}
				}
			}

			schema := db.schemaFor(f.Type)
			if _, isRef := schema["$ref"]; !isRef {
				applyValidateTag(schema, f.Tag.Get("validate"))
			}
			properties[name] = schema

			if !omitEmpty && hasValidateRule(f.Tag.Get("validate"), "required") {
				required = append(required, name)
			}
		}
	}
	visit(t)

	out := map[string]interface{}{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		sort.Strings(required)
		out["required"] = required
	}
	return out
}

func hasValidateRule(tag, name string) bool {
	if tag == "" {
		return false
	}
	for _, p := range strings.Split(tag, ",") {
		if p == name || strings.HasPrefix(p, name+"=") {
			return true
		}
	}
	return false
}

func applyValidateTag(schema map[string]interface{}, tag string) {
	if tag == "" {
		return
	}
	typ, _ := schema["type"].(string)
	for _, part := range strings.Split(tag, ",") {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch kv[0] {
		case "min":
			if n, err := strconv.Atoi(kv[1]); err == nil {
				switch typ {
				case "string":
					schema["minLength"] = n
				case "integer", "number":
					schema["minimum"] = n
				case "array":
					schema["minItems"] = n
				}
			}
		case "max":
			if n, err := strconv.Atoi(kv[1]); err == nil {
				switch typ {
				case "string":
					schema["maxLength"] = n
				case "integer", "number":
					schema["maximum"] = n
				case "array":
					schema["maxItems"] = n
				}
			}
		case "oneof":
			vals := strings.Fields(kv[1])
			enum := make([]interface{}, 0, len(vals))
			for _, v := range vals {
				enum = append(enum, v)
			}
			schema["enum"] = enum
		}
	}
}
