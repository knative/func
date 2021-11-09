package v06

import (
	"fmt"
	"os"

	"github.com/BurntSushi/toml"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/buildpack/layertypes"
)

type EncoderDecoder06 struct {
}

func NewEncoderDecoder() *EncoderDecoder06 {
	return &EncoderDecoder06{}
}

func (d *EncoderDecoder06) IsSupported(buildpackAPI string) bool {
	return api.MustParse(buildpackAPI).AtLeast("0.6")
}

func (d *EncoderDecoder06) Encode(file *os.File, lmf layertypes.LayerMetadataFile) error {
	// omit the types table - all the flags are set to false
	type dataTomlFile struct {
		Data interface{} `toml:"metadata"`
	}
	dtf := dataTomlFile{Data: lmf.Data}
	return toml.NewEncoder(file).Encode(dtf)
}

func (d *EncoderDecoder06) Decode(path string) (layertypes.LayerMetadataFile, string, error) {
	type typesTable struct {
		Build  bool `toml:"build"`
		Launch bool `toml:"launch"`
		Cache  bool `toml:"cache"`
	}
	type layerMetadataTomlFile struct {
		Data  interface{} `toml:"metadata"`
		Types typesTable  `toml:"types"`
	}

	var lmtf layerMetadataTomlFile
	md, err := toml.DecodeFile(path, &lmtf)
	if err != nil {
		return layertypes.LayerMetadataFile{}, "", err
	}
	msg := ""
	if isWrongFormat := typesInTopLevel(md); isWrongFormat {
		msg = fmt.Sprintf("the launch, cache and build flags should be in the types table of %s", path)
	}
	return layertypes.LayerMetadataFile{Data: lmtf.Data, Build: lmtf.Types.Build, Launch: lmtf.Types.Launch, Cache: lmtf.Types.Cache}, msg, nil
}

func typesInTopLevel(md toml.MetaData) bool {
	return md.IsDefined("build") || md.IsDefined("launch") || md.IsDefined("cache")
}
