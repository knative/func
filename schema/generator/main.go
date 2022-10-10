package main

import (
	"bytes"
	"encoding/json"
	"os"

	"github.com/alecthomas/jsonschema"

	fn "knative.dev/func"
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
// Genereated schema is written into schema/func_yaml-schema.json file
func generateFuncYamlSchema() error {
	// generate json schema for function struct
	js := jsonschema.Reflect(&fn.Function{})
	schema, err := js.MarshalJSON()
	if err != nil {
		return err
	}

	// indent the generated json
	var indentedSchema bytes.Buffer
	err = json.Indent(&indentedSchema, schema, "", "\t")
	if err != nil {
		return err
	}

	// write schema to the file
	return os.WriteFile("schema/func_yaml-schema.json", indentedSchema.Bytes(), 0644)
}
