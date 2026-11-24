package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"

	"github.com/alecthomas/jsonschema"

	fn "knative.dev/func/pkg/functions"
)

// This helper application generates json schemas:
// - schema for func.yaml stored in schema/func_yaml-schema.json
func main() {
	err := generateFuncYamlSchema()
	if err != nil {
		panic(err)
	}
}

// generateFuncYamlSchema generates json schema for function configuration file - func.yaml.
// Generated schema is written into schema/func_yaml-schema.json file
func generateFuncYamlSchema() error {
	schema, err := funcYamlSchema()
	if err != nil {
		return err
	}

	// write schema to the file
	return os.WriteFile("schema/func_yaml-schema.json", schema, 0644)
}

// funcYamlSchema generates the json schema for func.yaml exactly as it is
// written to schema/func_yaml-schema.json, indentation and trailing newline
// included. It is separated from generateFuncYamlSchema so the schema can be
// verified without writing to the working tree.
func funcYamlSchema() ([]byte, error) {
	// generate json schema for function struct
	r := &jsonschema.Reflector{}

	if err := r.AddGoComments("knative.dev/func", "./pkg/functions/"); err != nil {
		return nil, fmt.Errorf("cannot parse docstrings: %w", err)
	}

	js := r.Reflect(&fn.Function{})

	// restore the constraint reflection cannot represent
	if err := restoreTargetMinimum(js); err != nil {
		return nil, err
	}

	schema, err := js.MarshalJSON()
	if err != nil {
		return nil, err
	}

	// indent the generated json
	var indentedSchema bytes.Buffer
	if err = json.Indent(&indentedSchema, schema, "", "\t"); err != nil {
		return nil, err
	}

	if err = indentedSchema.WriteByte('\n'); err != nil {
		return nil, err
	}

	return indentedSchema.Bytes(), nil
}

// restoreTargetMinimum re-applies the fractional minimum declared on
// KPAScaleOptions.Target, which reflection truncates to zero.
//
// The reflector (github.com/alecthomas/jsonschema) represents the `minimum`
// keyword as an int and parses its tag value with strconv.Atoi, discarding the
// error:
//
//	type Type struct {
//		Minimum int `json:"minimum,omitempty"`
//	}
//
// KPAScaleOptions.Target declares jsonschema_extras:"minimum=0.01", so the
// value fails to parse, silently degrades to zero, and the generated schema
// ends up publishing "minimum": 0 where the runtime validation of the same
// value (validateKPAScale in pkg/functions/function_scale.go) requires
// >= 0.01.
//
// The value is read from the existing struct tag rather than duplicated in the
// generator, so this correction follows the schema constraint declared on
// KPAScaleOptions.Target.
func restoreTargetMinimum(schema *jsonschema.Schema) error {
	field, ok := reflect.TypeOf(fn.KPAScaleOptions{}).FieldByName("Target")
	if !ok {
		return fmt.Errorf("KPAScaleOptions has no Target field")
	}

	// The reflector names a property after the field's json tag, falling back
	// to its yaml tag; Target declares only a yaml name.
	name := strings.Split(field.Tag.Get("yaml"), ",")[0]
	definition, ok := schema.Definitions["KPAScaleOptions"]
	if !ok {
		return fmt.Errorf("generated schema has no KPAScaleOptions definition")
	}
	// orderedmap.Get dereferences its receiver, so a nil Properties map would
	// panic rather than report the malformed definition.
	if definition.Properties == nil {
		return fmt.Errorf("generated KPAScaleOptions definition has no properties")
	}
	property, ok := definition.Properties.Get(name)
	if !ok {
		return fmt.Errorf("KPAScaleOptions has no %q property", name)
	}
	t, ok := property.(*jsonschema.Type)
	if !ok {
		return fmt.Errorf("KPAScaleOptions.%s is not a schema", name)
	}

	for _, keyword := range strings.Split(field.Tag.Get("jsonschema_extras"), ",") {
		value, ok := strings.CutPrefix(keyword, "minimum=")
		if !ok {
			continue
		}
		minimum, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmt.Errorf("KPAScaleOptions.%s: minimum=%s is not a number", name, value)
		}
		if t.Extras == nil {
			t.Extras = map[string]interface{}{}
		}
		t.Extras["minimum"] = minimum
		return nil
	}
	return fmt.Errorf("KPAScaleOptions.%s declares no minimum", name)
}
