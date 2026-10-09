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
// place, because the component itself is being defined. A list of lists unwraps
// in a loop, and a struct field never holds an unregistered struct, so no step
// needs recursion.
func deriveType(kind reflect.Type, names map[reflect.Type]string, top bool) (Schema, error) {
	depth := 0
	for kind.Kind() == reflect.Pointer {
		kind = kind.Elem()
	}
	for kind.Kind() == reflect.Slice {
		depth++
		kind = kind.Elem()
		for kind.Kind() == reflect.Pointer {
			kind = kind.Elem()
		}
	}
	expand := false
	if top {
		expand = depth == 0
	}
	schema, err := deriveLeaf(kind, names, expand)
	if err != nil {
		return Schema{}, err
	}
	for ; depth > 0; depth-- {
		item := schema
		schema = Schema{Type: "array", Items: &item}
	}
	return schema, nil
}

// deriveLeaf builds the schema of a scalar, a map, or a struct.
func deriveLeaf(kind reflect.Type, names map[reflect.Type]string, expand bool) (Schema, error) {
	if name, registered := names[kind]; registered {
		if !expand {
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
// says omitempty, and a field tagged "-" is not part of the contract. A struct
// that is nested inside a field must be a registered document, which becomes a
// reference, so the derivation never descends into an unregistered struct.
func deriveStruct(kind reflect.Type, names map[reflect.Type]string) (Schema, error) {
	schema := Schema{Type: "object", Properties: map[string]Schema{}}
	for index := 0; index < kind.NumField(); index++ {
		field := kind.Field(index)
		name, optional, skip := jsonName(field)
		if skip {
			continue
		}
		if err := requireRegistered(field.Type, names); err != nil {
			return Schema{}, fmt.Errorf("%s.%s: %w", kind.Name(), field.Name, err)
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

// requireRegistered refuses a struct field whose struct type is not a registered
// document, because it would have to expand in place.
func requireRegistered(kind reflect.Type, names map[reflect.Type]string) error {
	for kind.Kind() == reflect.Pointer {
		kind = kind.Elem()
	}
	for kind.Kind() == reflect.Slice {
		kind = kind.Elem()
		for kind.Kind() == reflect.Pointer {
			kind = kind.Elem()
		}
	}
	if kind.Kind() != reflect.Struct {
		return nil
	}
	if _, registered := names[kind]; !registered {
		return fmt.Errorf("the nested type %s is not a registered document", kind)
	}
	return nil
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
