package k8s

import (
	mfc "github.com/manifestival/client-go-client"
	"github.com/manifestival/manifestival"
)

func GetManifestivalClient(c *Client) (manifestival.Client, error) {
	config, err := c.RestConfig()
	if err != nil {
		return nil, err
	}
	return mfc.NewClient(config)
}
