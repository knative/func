package main

import (
	"bytes"
	"encoding/json"
	"io/ioutil"

	"github.com/alecthomas/jsonschema"

	fn "knative.dev/kn-plugin-func"
)

// This helper application generates json schemas:
// - schema for func.yaml stored in schema/func_yaml-schema.json
func main() {
	err := generateFuncYamlSchema()
	if err != nil {
		panic(err)
	}
}

// generateFuncYamlSchema generates json schema for Function configuration file - func.yaml.
// Genereated schema is written into schema/func_yaml-schema.json file
func generateFuncYamlSchema() error {
	// generate json schema for Function struct
	js := jsonschema.Reflect(&fn.Config{})
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
	return ioutil.WriteFile("schema/func_yaml-schema.json", indentedSchema.Bytes(), 0644)
}
