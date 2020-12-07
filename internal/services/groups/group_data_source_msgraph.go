package groups

import (
	"context"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/manicminer/hamilton/models"

	"github.com/terraform-providers/terraform-provider-azuread/internal/clients"
	"github.com/terraform-providers/terraform-provider-azuread/internal/tf"
)

func groupDataSourceReadMsGraph(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*clients.Client).Groups.MsClient

	var group models.Group

	if objectId, ok := d.Get("object_id").(string); ok && objectId != "" {
		g, status, err := client.Get(ctx, objectId)
		if err != nil {
			if status == http.StatusNotFound {
				return tf.ErrorDiag(fmt.Sprintf("No group found with object ID: %q", objectId), "", "object_id")
			}
			return tf.ErrorDiag(fmt.Sprintf("Retrieving group with object ID: %q", objectId), err.Error(), "")
		}
		group = *g
	} else if displayName, ok := d.Get("name").(string); ok && displayName != "" {
		filter := fmt.Sprintf("displayName eq '%s'", displayName)
		groups, _, err := client.List(ctx, filter)
		if err != nil {
			return tf.ErrorDiag(fmt.Sprintf("No group found with display name: %q", displayName), err.Error(), "name")
		}

		count := len(*groups)
		if count > 1 {
			return tf.ErrorDiag(fmt.Sprintf("More than one group found with display name: %q", displayName), err.Error(), "name")
		} else if count == 0 {
			return tf.ErrorDiag(fmt.Sprintf("No group found with display name: %q", displayName), err.Error(), "name")
		}

		group = (*groups)[0]
	} else {
		return tf.ErrorDiag("One of `object_id` or `name` must be specified", "", "")
	}

	if group.ID == nil {
		return tf.ErrorDiag("Bad API response", "API returned group with nil object ID", "")
	}

	d.SetId(*group.ID)

	if err := d.Set("object_id", group.ID); err != nil {
		return tf.ErrorDiag("Could not set attribute", err.Error(), "object_id")
	}

	if err := d.Set("name", group.DisplayName); err != nil {
		return tf.ErrorDiag("Could not set attribute", err.Error(), "name")
	}

	if err := d.Set("description", group.Description); err != nil {
		return tf.ErrorDiag("Could not set attribute", err.Error(), "description")
	}

	members, _, err := client.ListMembers(ctx, d.Id())
	if err != nil {
		return tf.ErrorDiag(fmt.Sprintf("Could not retrieve group members for group with object ID: %q", d.Id()), err.Error(), "")
	}

	if err := d.Set("members", members); err != nil {
		return tf.ErrorDiag("Could not set attribute", err.Error(), "members")
	}

	owners, _, err := client.ListOwners(ctx, d.Id())
	if err != nil {
		return tf.ErrorDiag(fmt.Sprintf("Could not retrieve group owners for group with object ID: %q", d.Id()), err.Error(), "")
	}

	if err := d.Set("owners", owners); err != nil {
		return tf.ErrorDiag("Could not set attribute", err.Error(), "owners")
	}

	return nil
}
