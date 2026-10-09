package openapi

import (
	"fmt"
	"reflect"
	"strings"
)

// deriveSchema builds the JSON Schema of one Go type from its JSON tags. The
// published document derives its schemas from the shared wire types, so a field
// the server returns cannot be missing from the document. A named type that is
// registered in names becomes a reference when it appears inside another schema.
func deriveSchema(sample any, description string, names map[reflect.Type]string) (Schema, error) {
	kind := reflect.TypeOf(sample)
	if kind == nil {
		return Schema{}, fmt.Errorf("a schema was derived from no type")
	}
	schema, err := deriveType(kind, names, true)
	if err != nil {
		return Schema{}, err
	}
	schema.Description = description
	return schema, nil
}

// deriveType builds one schema. The top flag makes a registered struct expand in
// place, because the component itself is being defined.
func deriveType(kind reflect.Type, names map[reflect.Type]string, top bool) (Schema, error) {
	for kind.Kind() == reflect.Pointer {
		kind = kind.Elem()
	}
	if name, registered := names[kind]; registered {
		if !top {
			return refSchema(name), nil
		}
	}
	switch kind.Kind() {
	case reflect.String:
		return Schema{Type: "string"}, nil
	case reflect.Bool:
		return Schema{Type: "boolean"}, nil
	case reflect.Int, reflect.Int32, reflect.Int64, reflect.Uint32, reflect.Uint64:
		return Schema{Type: "integer", Format: "int64"}, nil
	case reflect.Slice:
		item, err := deriveType(kind.Elem(), names, false)
		if err != nil {
			return Schema{}, err
		}
		return Schema{Type: "array", Items: &item}, nil
	case reflect.Map:
		if kind.Key().Kind() != reflect.String {
			return Schema{}, fmt.Errorf("the map type %s has a key that is not a string", kind)
		}
		return Schema{Type: "object", AdditionalProperties: true}, nil
	case reflect.Struct:
		return deriveStruct(kind, names)
	}
	return Schema{}, fmt.Errorf("the type %s has no JSON Schema form", kind)
}

// deriveStruct builds an object schema. A field is required unless its JSON tag
// says omitempty, and a field tagged "-" is not part of the contract.
func deriveStruct(kind reflect.Type, names map[reflect.Type]string) (Schema, error) {
	schema := Schema{Type: "object", Properties: map[string]Schema{}}
	for index := 0; index < kind.NumField(); index++ {
		field := kind.Field(index)
		name, optional, skip := jsonName(field)
		if skip {
			continue
		}
		property, err := deriveType(field.Type, names, false)
		if err != nil {
			return Schema{}, fmt.Errorf("%s.%s: %w", kind.Name(), field.Name, err)
		}
		schema.Properties[name] = property
		if !optional {
			schema.Required = append(schema.Required, name)
		}
	}
	if len(schema.Properties) == 0 {
		return Schema{}, fmt.Errorf("the type %s declares no JSON field", kind)
	}
	return schema, nil
}

// jsonName reads a field's JSON name and whether it may be absent.
func jsonName(field reflect.StructField) (name string, optional, skip bool) {
	if !field.IsExported() {
		return "", false, true
	}
	tag := field.Tag.Get("json")
	if tag == "-" {
		return "", false, true
	}
	name, options, _ := strings.Cut(tag, ",")
	if name == "" {
		name = field.Name
	}
	return name, strings.Contains(options, "omitempty"), false
}
