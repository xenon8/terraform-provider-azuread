package clients

import (
	"context"

	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure"

	applications "github.com/terraform-providers/terraform-provider-azuread/internal/services/applications/client"
	domains "github.com/terraform-providers/terraform-provider-azuread/internal/services/domains/client"
	groups "github.com/terraform-providers/terraform-provider-azuread/internal/services/groups/client"
	serviceprincipals "github.com/terraform-providers/terraform-provider-azuread/internal/services/serviceprincipals/client"
	users "github.com/terraform-providers/terraform-provider-azuread/internal/services/users/client"
)

// Client contains the handles to all the specific Azure AD resource classes' respective clients
type Client struct {
	ClientID         string
	ObjectID         string
	TenantID         string
	TerraformVersion string
	Environment      azure.Environment

	AuthenticatedAsAServicePrincipal bool

	StopContext context.Context

	Applications      *applications.Client
	Domains           *domains.Client
	Groups            *groups.Client
	ServicePrincipals *serviceprincipals.Client
	Users             *users.Client
}

func (client *Client) build(ctx context.Context, o *clientOptions) error {
	autorest.Count429AsRetry = false
	client.StopContext = ctx

	client.Applications = applications.BuildClient(o)
	client.Domains = domains.BuildClient(o)
	client.Groups = groups.BuildClient(o)
	client.ServicePrincipals = serviceprincipals.BuildClient(o)
	client.Users = users.BuildClient(o)

	return nil
}
