package groups

import (
	"context"
	"errors"
	"log"
	"net/http"

	"github.com/hashicorp/go-uuid"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/manicminer/hamilton/models"

	"github.com/terraform-providers/terraform-provider-azuread/internal/clients"
	"github.com/terraform-providers/terraform-provider-azuread/internal/helpers/msgraph"
	"github.com/terraform-providers/terraform-provider-azuread/internal/tf"
	"github.com/terraform-providers/terraform-provider-azuread/internal/utils"
)

func groupResourceCreateMsGraph(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*clients.Client).Groups.MsClient

	displayName := d.Get("name").(string)

	if d.Get("prevent_duplicate_names").(bool) {
		existingId, err := msgraph.GroupCheckNameAvailability(ctx, client, displayName, nil)
		if err != nil {
			return tf.ErrorDiagPathF(err, "name", "Could not check for existing group(s)")
		}
		if existingId != nil {
			return tf.ImportAsDuplicateDiag("azuread_group", *existingId, displayName)
		}
	}

	mailNickname, err := uuid.GenerateUUID()
	if err != nil {
		return tf.ErrorDiagF(err, "Failed to generate mailNickname")
	}

	properties := models.Group{
		DisplayName:  utils.String(displayName),
		MailNickname: utils.String(mailNickname),

		// API only supports creation of security groups
		SecurityEnabled: utils.Bool(true),
		MailEnabled:     utils.Bool(false),
	}

	if v, ok := d.GetOk("description"); ok {
		properties.Description = utils.String(v.(string))
	}

	if v, ok := d.GetOk("members"); ok {
		members := v.(*schema.Set).List()
		for _, o := range members {
			properties.AppendMember(client.BaseClient.Endpoint, client.BaseClient.ApiVersion, o.(string))
		}
	}

	if v, ok := d.GetOk("owners"); ok {
		owners := v.(*schema.Set).List()
		for _, o := range owners {
			properties.AppendOwner(client.BaseClient.Endpoint, client.BaseClient.ApiVersion, o.(string))
		}
	}

	group, _, err := client.Create(ctx, properties)
	if err != nil {
		return tf.ErrorDiagF(err, "Creating group %q", displayName)
	}

	if group.ID == nil {
		return tf.ErrorDiagF(errors.New("API returned group with nil object ID"), "Bad API Response")
	}

	d.SetId(*group.ID)

	_, err = msgraph.WaitForCreationReplication(ctx, d.Timeout(schema.TimeoutCreate), func() (interface{}, int, error) {
		return client.Get(ctx, *group.ID)
	})

	if err != nil {
		return tf.ErrorDiagF(err, "Waiting for Group with object ID: %q", *group.ID)
	}

	return groupResourceReadMsGraph(ctx, d, meta)
}

func groupResourceReadMsGraph(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*clients.Client).Groups.MsClient

	group, status, err := client.Get(ctx, d.Id())
	if err != nil {
		if status == http.StatusNotFound {
			log.Printf("[DEBUG] Group with ID %q was not found - removing from state", d.Id())
			d.SetId("")
			return nil
		}
		return tf.ErrorDiagF(err, "Retrieving group with object ID: %q", d.Id())
	}

	if dg := tf.Set(d, "object_id", group.ID); dg != nil {
		return dg
	}

	if dg := tf.Set(d, "name", group.DisplayName); dg != nil {
		return dg
	}

	if dg := tf.Set(d, "description", group.Description); dg != nil {
		return dg
	}

	owners, _, err := client.ListOwners(ctx, *group.ID)
	if err != nil {
		return tf.ErrorDiagPathF(err, "owners", "Could not retrieve owners for group with object ID %q", d.Id())
	}

	if dg := tf.Set(d, "owners", owners); dg != nil {
		return dg
	}

	members, _, err := client.ListMembers(ctx, *group.ID)
	if err != nil {
		return tf.ErrorDiagPathF(err, "owners", "Could not retrieve members for group with object ID %q", d.Id())
	}

	if dg := tf.Set(d, "members", members); dg != nil {
		return dg
	}

	preventDuplicates := false
	if v := d.Get("prevent_duplicate_names").(bool); v {
		preventDuplicates = v
	}

	if dg := tf.Set(d, "prevent_duplicate_names", preventDuplicates); dg != nil {
		return dg
	}

	return nil
}

func groupResourceUpdateMsGraph(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*clients.Client).Groups.MsClient
	group := models.Group{ID: utils.String(d.Id())}
	displayName := d.Get("name").(string)

	if d.HasChange("display_name") {
		if preventDuplicates := d.Get("prevent_duplicate_names").(bool); preventDuplicates {
			existingId, err := msgraph.GroupCheckNameAvailability(ctx, client, displayName, group.ID)
			if err != nil {
				return tf.ErrorDiagPathF(err, "name", "Could not check for existing group(s)")
			}
			if existingId != nil {
				return tf.ImportAsDuplicateDiag("azuread_group", *existingId, displayName)
			}
		}

		group.DisplayName = utils.String(displayName)
	}

	if d.HasChange("description") {
		group.Description = utils.String(d.Get("description").(string))
	}

	if _, err := client.Update(ctx, group); err != nil {
		return tf.ErrorDiagF(err, "Updating group with ID: %q", d.Id())
	}

	if v, ok := d.GetOkExists("members"); ok && d.HasChange("members") { //nolint:SA1019
		members, _, err := client.ListMembers(ctx, *group.ID)
		if err != nil {
			return tf.ErrorDiagF(err, "Could not retrieve members for group with ID: %q", d.Id())
		}

		existingMembers := *members
		desiredMembers := *tf.ExpandStringSlicePtr(v.(*schema.Set).List())
		membersForRemoval := utils.Difference(existingMembers, desiredMembers)
		membersToAdd := utils.Difference(desiredMembers, existingMembers)

		if membersForRemoval != nil {
			if _, err = client.RemoveMembers(ctx, d.Id(), &membersForRemoval); err != nil {
				return tf.ErrorDiagF(err, "Could not remove members from group with ID: %q", d.Id())
			}
		}

		if membersToAdd != nil {
			for _, m := range membersToAdd {
				group.AppendMember(client.BaseClient.Endpoint, client.BaseClient.ApiVersion, m)
			}

			if _, err := client.AddMembers(ctx, &group); err != nil {
				return tf.ErrorDiagF(err, "Could not add members to group with ID: %q", d.Id())
			}
		}
	}

	if v, ok := d.GetOkExists("owners"); ok && d.HasChange("owners") { //nolint:SA1019
		owners, _, err := client.ListOwners(ctx, *group.ID)
		if err != nil {
			return tf.ErrorDiagF(err, "Could not retrieve eowners for group with ID: %q", d.Id())
		}

		existingOwners := *owners
		desiredOwners := *tf.ExpandStringSlicePtr(v.(*schema.Set).List())
		ownersForRemoval := utils.Difference(existingOwners, desiredOwners)
		ownersToAdd := utils.Difference(desiredOwners, existingOwners)

		if ownersForRemoval != nil {
			if _, err = client.RemoveOwners(ctx, d.Id(), &ownersForRemoval); err != nil {
				return tf.ErrorDiagF(err, "Could not remove owners from group with ID: %q", d.Id())
			}
		}

		if ownersToAdd != nil {
			for _, m := range ownersToAdd {
				group.AppendOwner(client.BaseClient.Endpoint, client.BaseClient.ApiVersion, m)
			}

			if _, err := client.AddOwners(ctx, &group); err != nil {
				return tf.ErrorDiagF(err, "Could not add owners to group with ID: %q", d.Id())
			}
		}
	}

	return groupResourceReadMsGraph(ctx, d, meta)
}

func groupResourceDeleteMsGraph(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*clients.Client).Groups.MsClient

	if _, err := client.Delete(ctx, d.Id()); err != nil {
		return tf.ErrorDiagF(err, "Deleting group with object ID: %q", d.Id())
	}

	return nil
}
