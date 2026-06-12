package k8s

import (
	mfc "github.com/manifestival/client-go-client"
	"github.com/manifestival/manifestival"
)

func GetManifestivalClient(kc *Client) (manifestival.Client, error) {
	config, err := kc.ClientConfig()
	if err != nil {
		return nil, err
	}
	return mfc.NewClient(config)
}
