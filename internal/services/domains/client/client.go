package client

import (
	"github.com/Azure/azure-sdk-for-go/services/graphrbac/1.6/graphrbac"
	"github.com/terraform-providers/terraform-provider-azuread/internal/services"
)

type Client struct {
	DomainsClient *graphrbac.DomainsClient
}

func BuildClient(o *services.ClientOptions) *Client {
	domainsClient := graphrbac.NewDomainsClientWithBaseURI(o.AadGraphEndpoint, o.TenantID)
	o.ConfigureClient(&domainsClient.Client, o.AadGraphAuthorizer)

	return &Client{
		DomainsClient: &domainsClient,
	}
}
