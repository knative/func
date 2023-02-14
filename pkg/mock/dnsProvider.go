package mock

type DNSProvider struct {
	ProvideInvoked   bool
	NameRequested    string
	AddressRequested string
}

func NewDNSProvider() *DNSProvider {
	return &DNSProvider{}
}

func (d *DNSProvider) Provide(name, address string) string {
	d.ProvideInvoked = true
	d.NameRequested = name
	d.AddressRequested = address
	return name
}
