package client

import (
	"github.com/Azure/azure-sdk-for-go/services/graphrbac/1.6/graphrbac"
	"github.com/terraform-providers/terraform-provider-azuread/internal/services"
)

type Client struct {
	ServicePrincipalsClient *graphrbac.ServicePrincipalsClient
}

func BuildClient(o *services.ClientOptions) *Client {
	servicePrincipalsClient := graphrbac.NewServicePrincipalsClientWithBaseURI(o.AadGraphEndpoint, o.TenantID)
	o.ConfigureClient(&servicePrincipalsClient.Client, o.AadGraphAuthorizer)

	return &Client{
		ServicePrincipalsClient: &servicePrincipalsClient,
	}
}
