package client

import (
	"github.com/Azure/azure-sdk-for-go/services/graphrbac/1.6/graphrbac"
	"github.com/terraform-providers/terraform-provider-azuread/internal/services"
)

type Client struct {
	UsersClient *graphrbac.UsersClient
}

func BuildClient(o *services.ClientOptions) *Client {
	usersClient := graphrbac.NewUsersClientWithBaseURI(o.AadGraphEndpoint, o.TenantID)
	o.ConfigureClient(&usersClient.Client, o.AadGraphAuthorizer)

	return &Client{
		UsersClient: &usersClient,
	}
}
