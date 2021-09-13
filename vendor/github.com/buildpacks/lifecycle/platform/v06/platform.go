package v06

type Platform struct {
	api string
}

func NewPlatform(apiStr string) *Platform {
	return &Platform{api: apiStr}
}

func (p *Platform) API() string {
	return p.api
}
