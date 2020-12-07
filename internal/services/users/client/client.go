package client

import (
	"github.com/Azure/azure-sdk-for-go/services/graphrbac/1.6/graphrbac"
	"github.com/terraform-providers/terraform-provider-azuread/internal/common"
)

type Client struct {
	UsersClient *graphrbac.UsersClient
}

func BuildClient(o *common.ClientOptions) *Client {
	usersClient := graphrbac.NewUsersClientWithBaseURI(o.AadGraphEndpoint, o.TenantID)
	o.ConfigureClient(&usersClient.Client, o.AadGraphAuthorizer)

	return &Client{
		UsersClient: &usersClient,
	}
}
