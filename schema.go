package aegis

import (
	"reflect"
	"strings"
)

type Schema struct {
	Type                 string            `json:"type,omitempty"`
	Description          string            `json:"description,omitempty"`
	Properties           map[string]Schema `json:"properties,omitempty"`
	Items                *Schema           `json:"items,omitempty"`
	Required             []string          `json:"required,omitempty"`
	Enum                 []string          `json:"enum,omitempty"`
	AdditionalProperties bool              `json:"additionalProperties,omitempty"`
}

func SchemaOf[T any]() Schema {
	var zero *T
	return schemaFromType(reflect.TypeOf(zero).Elem())
}

func schemaFromType(t reflect.Type) Schema {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	switch t.Kind() {
	case reflect.Bool:
		return Schema{Type: "boolean"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return Schema{Type: "integer"}
	case reflect.Float32, reflect.Float64:
		return Schema{Type: "number"}
	case reflect.String:
		return Schema{Type: "string"}
	case reflect.Slice:
		if t.Elem().Kind() == reflect.Uint8 {
			return Schema{Type: "string", Description: "binary"}
		}
		item := schemaFromType(t.Elem())
		return Schema{Type: "array", Items: &item}
	case reflect.Array:
		item := schemaFromType(t.Elem())
		return Schema{Type: "array", Items: &item}
	case reflect.Map:
		return Schema{Type: "object", AdditionalProperties: true}
	case reflect.Struct:
		properties := map[string]Schema{}
		required := make([]string, 0, t.NumField())

		for index := 0; index < t.NumField(); index++ {
			field := t.Field(index)
			if !field.IsExported() {
				continue
			}

			name, optional, skip := schemaFieldName(field)
			if skip {
				continue
			}

			properties[name] = schemaFromType(field.Type)
			if !optional {
				required = append(required, name)
			}
		}

		out := Schema{
			Type:       "object",
			Properties: properties,
		}
		if len(required) > 0 {
			out.Required = required
		}
		return out
	default:
		return Schema{Type: "object", AdditionalProperties: true}
	}
}

func schemaFieldName(field reflect.StructField) (name string, optional bool, skip bool) {
	tag := field.Tag.Get("json")
	if tag == "-" {
		return "", false, true
	}

	name = field.Name
	if tag != "" {
		parts := strings.Split(tag, ",")
		if parts[0] != "" {
			name = parts[0]
		}
		for _, part := range parts[1:] {
			if part == "omitempty" {
				optional = true
			}
		}
	}

	for field.Type.Kind() == reflect.Pointer {
		field.Type = field.Type.Elem()
		optional = true
	}

	return name, optional, false
}

func schemaSummary(schema Schema) string {
	switch schema.Type {
	case "object":
		if len(schema.Properties) == 0 {
			return "object"
		}
		keys := make([]string, 0, len(schema.Properties))
		for key := range schema.Properties {
			keys = append(keys, key)
		}
		return "object with " + strings.Join(keys, ", ")
	case "array":
		return "array"
	case "":
		return "value"
	default:
		return schema.Type
	}
}
