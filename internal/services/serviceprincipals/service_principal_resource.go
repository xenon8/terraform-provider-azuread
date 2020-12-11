package serviceprincipals

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/Azure/azure-sdk-for-go/services/graphrbac/1.6/graphrbac"
	"github.com/hashicorp/go-azure-helpers/response"
	"github.com/hashicorp/go-uuid"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

	"github.com/terraform-providers/terraform-provider-azuread/internal/clients"
	"github.com/terraform-providers/terraform-provider-azuread/internal/helpers/aadgraph"
	"github.com/terraform-providers/terraform-provider-azuread/internal/tf"
	"github.com/terraform-providers/terraform-provider-azuread/internal/utils"
	"github.com/terraform-providers/terraform-provider-azuread/internal/validate"
)

const servicePrincipalResourceName = "azuread_service_principal"

func servicePrincipalResource() *schema.Resource {
	return &schema.Resource{
		CreateContext: servicePrincipalResourceCreate,
		ReadContext:   servicePrincipalResourceRead,
		UpdateContext: servicePrincipalResourceUpdate,
		DeleteContext: servicePrincipalResourceDelete,

		Importer: tf.ValidateResourceIDPriorToImport(func(id string) error {
			if _, err := uuid.ParseUUID(id); err != nil {
				return fmt.Errorf("specified ID (%q) is not valid: %s", id, err)
			}
			return nil
		}),

		Schema: map[string]*schema.Schema{
			"application_id": {
				Type:             schema.TypeString,
				Required:         true,
				ForceNew:         true,
				ValidateDiagFunc: validate.UUID,
			},

			"app_role_assignment_required": {
				Type:     schema.TypeBool,
				Optional: true,
			},

			"display_name": {
				Type:     schema.TypeString,
				Computed: true,
			},

			"object_id": {
				Type:     schema.TypeString,
				Computed: true,
			},

			"app_roles": schemaAppRolesComputed(),

			"oauth2_permissions": schemaOauth2PermissionsComputed(),

			"tags": {
				Type:     schema.TypeSet,
				Optional: true,
				Set:      schema.HashString,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},
		},
	}
}

func servicePrincipalResourceCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*clients.Client).ServicePrincipals.ServicePrincipalsClient

	applicationId := d.Get("application_id").(string)

	properties := graphrbac.ServicePrincipalCreateParameters{
		AppID: utils.String(applicationId),
		// there's no way of retrieving this, and there's no way of changing it
		// given there's no way to change it - we'll just default this to true
		AccountEnabled: utils.Bool(true),
	}

	if v, ok := d.GetOk("app_role_assignment_required"); ok {
		properties.AppRoleAssignmentRequired = utils.Bool(v.(bool))
	}

	if v, ok := d.GetOk("tags"); ok {
		properties.Tags = tf.ExpandStringSlicePtr(v.(*schema.Set).List())
	}

	sp, err := client.Create(ctx, properties)
	if err != nil {
		return tf.ErrorDiagF(err, "Could not create service principal")
	}
	if sp.ObjectID == nil || *sp.ObjectID == "" {
		return tf.ErrorDiagF(errors.New("ObjectID returned for service principal is nil"), "Bad API response")
	}
	d.SetId(*sp.ObjectID)

	_, err = aadgraph.WaitForCreationReplication(ctx, d.Timeout(schema.TimeoutCreate), func() (interface{}, error) {
		return client.Get(ctx, *sp.ObjectID)
	})
	if err != nil {
		return tf.ErrorDiagF(err, "Waiting for service principal with object ID: %q", *sp.ObjectID)
	}

	return servicePrincipalResourceRead(ctx, d, meta)
}

func servicePrincipalResourceUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*clients.Client).ServicePrincipals.ServicePrincipalsClient

	var properties graphrbac.ServicePrincipalUpdateParameters

	if d.HasChange("app_role_assignment_required") {
		properties.AppRoleAssignmentRequired = utils.Bool(d.Get("app_role_assignment_required").(bool))
	}

	if d.HasChange("tags") {
		if v, ok := d.GetOk("tags"); ok {
			properties.Tags = tf.ExpandStringSlicePtr(v.(*schema.Set).List())
		} else {
			empty := []string{} // clear tags with empty array
			properties.Tags = &empty
		}
	}

	if _, err := client.Update(ctx, d.Id(), properties); err != nil {
		return tf.ErrorDiagF(err, "Updating service principal with object ID: %q", d.Id())
	}

	// Wait for replication delay after updating
	_, err := aadgraph.WaitForCreationReplication(ctx, d.Timeout(schema.TimeoutCreate), func() (interface{}, error) {
		return client.Get(ctx, d.Id())
	})
	if err != nil {
		return tf.ErrorDiagF(err, "Waiting for service principal with object ID: %q", d.Id())
	}

	return servicePrincipalResourceRead(ctx, d, meta)
}

func servicePrincipalResourceRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*clients.Client).ServicePrincipals.ServicePrincipalsClient

	objectId := d.Id()

	sp, err := client.Get(ctx, objectId)
	if err != nil {
		if utils.ResponseWasNotFound(sp.Response) {
			log.Printf("[DEBUG] Service Principal with Object ID %q was not found - removing from state!", objectId)
			d.SetId("")
			return nil
		}

		return tf.ErrorDiagF(err, "retrieving service principal with object ID: %q", d.Id())
	}

	if dg := tf.Set(d, "object_id", sp.ObjectID); dg != nil {
		return dg
	}

	if dg := tf.Set(d, "application_id", sp.AppID); dg != nil {
		return dg
	}

	if dg := tf.Set(d, "display_name", sp.DisplayName); dg != nil {
		return dg
	}

	if dg := tf.Set(d, "app_role_assignment_required", sp.AppRoleAssignmentRequired); dg != nil {
		return dg
	}

	if dg := tf.Set(d, "tags", sp.Tags); dg != nil {
		return dg
	}

	if dg := tf.Set(d, "app_roles", aadgraph.FlattenAppRoles(sp.AppRoles)); dg != nil {
		return dg
	}

	if dg := tf.Set(d, "oauth2_permissions", aadgraph.FlattenOauth2Permissions(sp.Oauth2Permissions)); dg != nil {
		return dg
	}

	return nil
}

func servicePrincipalResourceDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*clients.Client).ServicePrincipals.ServicePrincipalsClient

	applicationId := d.Id()
	app, err := client.Delete(ctx, applicationId)
	if err != nil {
		if !response.WasNotFound(app.Response) {
			return tf.ErrorDiagF(err, "Deleting service principal with object ID: %q", d.Id())
		}
	}

	return nil
}
