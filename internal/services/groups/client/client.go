package client

import (
	"github.com/Azure/azure-sdk-for-go/services/graphrbac/1.6/graphrbac"
	"github.com/terraform-providers/terraform-provider-azuread/internal/services"
)

type Client struct {
	GroupsClient *graphrbac.GroupsClient
}

func BuildClient(o *services.ClientOptions) *Client {
	groupsClient := graphrbac.NewGroupsClientWithBaseURI(o.AadGraphEndpoint, o.TenantID)
	o.ConfigureClient(&groupsClient.Client, o.AadGraphAuthorizer)

	return &Client{
		GroupsClient: &groupsClient,
	}
}
