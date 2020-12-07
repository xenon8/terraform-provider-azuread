package client

import (
	"github.com/Azure/azure-sdk-for-go/services/graphrbac/1.6/graphrbac"
	"github.com/terraform-providers/terraform-provider-azuread/internal/services"
)

type Client struct {
	ApplicationsClient *graphrbac.ApplicationsClient
}

func BuildClient(o *services.ClientOptions) *Client {
	applicationsClient := graphrbac.NewApplicationsClientWithBaseURI(o.AadGraphEndpoint, o.TenantID)
	o.ConfigureClient(&applicationsClient.Client, o.AadGraphAuthorizer)

	return &Client{
		ApplicationsClient: &applicationsClient,
	}
}
