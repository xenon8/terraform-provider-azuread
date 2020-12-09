package groups

import (
	"context"
	"errors"
	"log"

	"github.com/Azure/azure-sdk-for-go/services/graphrbac/1.6/graphrbac"
	"github.com/hashicorp/go-uuid"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

	"github.com/terraform-providers/terraform-provider-azuread/internal/clients"
	"github.com/terraform-providers/terraform-provider-azuread/internal/helpers/aadgraph"
	"github.com/terraform-providers/terraform-provider-azuread/internal/tf"
	"github.com/terraform-providers/terraform-provider-azuread/internal/utils"
)

func groupResourceCreateAadGraph(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*clients.Client).Groups.AadClient

	name := d.Get("name").(string)

	if d.Get("prevent_duplicate_names").(bool) {
		existingGroup, err := aadgraph.GroupFindByName(ctx, client, name)
		if err != nil {
			return tf.ErrorDiagPathF(err, "name", "Could not check for existing group(s)")
		}
		if existingGroup != nil {
			if existingGroup.ObjectID == nil {
				return tf.ImportAsDuplicateDiag("azuread_group", "unknown", name)
			}
			return tf.ImportAsDuplicateDiag("azuread_group", *existingGroup.ObjectID, name)
		}
	}

	mailNickname, err := uuid.GenerateUUID()
	if err != nil {
		return tf.ErrorDiagF(err, "Failed to generate mailNickname")
	}

	properties := graphrbac.GroupCreateParameters{
		DisplayName:          &name,
		MailEnabled:          utils.Bool(false),          // we're defaulting to false, as the API currently only supports the creation of non-mail enabled security groups.
		MailNickname:         utils.String(mailNickname), // this matches the portal behaviour
		SecurityEnabled:      utils.Bool(true),           // we're defaulting to true, as the API currently only supports the creation of non-mail enabled security groups.
		AdditionalProperties: make(map[string]interface{}),
	}

	if v, ok := d.GetOk("description"); ok {
		properties.AdditionalProperties["description"] = v.(string)
	}

	group, err := client.Create(ctx, properties)
	if err != nil {
		return tf.ErrorDiagF(err, "Creating group %q", name)
	}

	if group.ObjectID == nil || *group.ObjectID == "" {
		return tf.ErrorDiagF(errors.New("API returned group with nil object ID"), "Bad API Response")
	}

	d.SetId(*group.ObjectID)

	_, err = aadgraph.WaitForCreationReplication(ctx, d.Timeout(schema.TimeoutCreate), func() (interface{}, error) {
		return client.Get(ctx, *group.ObjectID)
	})

	if err != nil {
		return tf.ErrorDiagF(err, "Waiting for Group with object ID: %q", *group.ObjectID)
	}

	// Add members if specified
	if v, ok := d.GetOk("members"); ok {
		members := tf.ExpandStringSlicePtr(v.(*schema.Set).List())

		// we could lock here against the group member resource, but they should not be used together (todo conflicts with at a resource level?)
		if err := aadgraph.GroupAddMembers(ctx, client, *group.ObjectID, *members); err != nil {
			return tf.ErrorDiagF(err, "Adding group members")
		}
	}

	// Add owners if specified
	if v, ok := d.GetOk("owners"); ok {
		existingOwners, err := aadgraph.GroupAllOwners(ctx, client, *group.ObjectID)
		if err != nil {
			return tf.ErrorDiagF(err, "Could not retrieve group owners")
		}
		members := *tf.ExpandStringSlicePtr(v.(*schema.Set).List())
		ownersToAdd := utils.Difference(members, existingOwners)

		if err := aadgraph.GroupAddOwners(ctx, client, *group.ObjectID, ownersToAdd); err != nil {
			return tf.ErrorDiagF(err, "Adding group owners")
		}
	}

	return groupResourceReadAadGraph(ctx, d, meta)
}

func groupResourceReadAadGraph(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*clients.Client).Groups.AadClient

	resp, err := client.Get(ctx, d.Id())
	if err != nil {
		if utils.ResponseWasNotFound(resp.Response) {
			log.Printf("[DEBUG] Group with id %q was not found - removing from state", d.Id())
			d.SetId("")
			return nil
		}

		return tf.ErrorDiagF(err, "Retrieving group with object ID: %q", d.Id())
	}

	if dg := tf.Set(d, "object_id", resp.ObjectID); dg != nil {
		return dg
	}

	if dg := tf.Set(d, "name", resp.DisplayName); dg != nil {
		return dg
	}

	description := ""
	if v, ok := resp.AdditionalProperties["description"]; ok {
		description = v.(string)
	}
	if dg := tf.Set(d, "description", description); dg != nil {
		return dg
	}

	members, err := aadgraph.GroupAllMembers(ctx, client, d.Id())
	if err != nil {
		return tf.ErrorDiagPathF(err, "owners", "Could not retrieve members for group with object ID %q", d.Id())
	}
	if dg := tf.Set(d, "members", members); dg != nil {
		return dg
	}

	owners, err := aadgraph.GroupAllOwners(ctx, client, d.Id())
	if err != nil {
		return tf.ErrorDiagPathF(err, "owners", "Could not retrieve owners for group with object ID %q", d.Id())
	}
	if dg := tf.Set(d, "owners", owners); dg != nil {
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

func groupResourceUpdateAadGraph(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*clients.Client).Groups.AadClient

	if v, ok := d.GetOkExists("members"); ok && d.HasChange("members") { //nolint:SA1019
		existingMembers, err := aadgraph.GroupAllMembers(ctx, client, d.Id())
		if err != nil {
			return tf.ErrorDiagPathF(err, "owners", "Could not retrieve members for group with object ID %q", d.Id())
		}

		desiredMembers := *tf.ExpandStringSlicePtr(v.(*schema.Set).List())
		membersForRemoval := utils.Difference(existingMembers, desiredMembers)
		membersToAdd := utils.Difference(desiredMembers, existingMembers)

		for _, existingMember := range membersForRemoval {
			log.Printf("[DEBUG] Removing member with id %q from Group with id %q", existingMember, d.Id())
			if err := aadgraph.GroupRemoveMember(ctx, client, d.Timeout(schema.TimeoutDelete), d.Id(), existingMember); err != nil {
				return tf.ErrorDiagF(err, "Removing group members")
			}

			if _, err := aadgraph.WaitForListRemove(ctx, existingMember, func() ([]string, error) {
				return aadgraph.GroupAllMembers(ctx, client, d.Id())
			}); err != nil {
				return tf.ErrorDiagF(err, "Waiting for group membership removal")
			}
		}

		if err := aadgraph.GroupAddMembers(ctx, client, d.Id(), membersToAdd); err != nil {
			return tf.ErrorDiagF(err, "Adding group members")
		}
	}

	if v, ok := d.GetOkExists("owners"); ok && d.HasChange("owners") { //nolint:SA1019
		existingOwners, err := aadgraph.GroupAllOwners(ctx, client, d.Id())
		if err != nil {
			return tf.ErrorDiagPathF(err, "owners", "Could not retrieve owners for group with object ID %q", d.Id())
		}

		desiredOwners := *tf.ExpandStringSlicePtr(v.(*schema.Set).List())
		ownersForRemoval := utils.Difference(existingOwners, desiredOwners)
		ownersToAdd := utils.Difference(desiredOwners, existingOwners)

		for _, ownerToDelete := range ownersForRemoval {
			log.Printf("[DEBUG] Removing member with ID %q from Group with ID %q", ownerToDelete, d.Id())
			if resp, err := client.RemoveOwner(ctx, d.Id(), ownerToDelete); err != nil {
				if !utils.ResponseWasNotFound(resp) {
					return tf.ErrorDiagF(err, "Removing group owner %q from group with object ID: %q", ownerToDelete, d.Id())
				}
			}
		}

		if err := aadgraph.GroupAddOwners(ctx, client, d.Id(), ownersToAdd); err != nil {
			return tf.ErrorDiagF(err, "Adding group owners")
		}
	}

	return groupResourceRead(ctx, d, meta)
}

func groupResourceDeleteAadGraph(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*clients.Client).Groups.AadClient

	if resp, err := client.Delete(ctx, d.Id()); err != nil {
		if !utils.ResponseWasNotFound(resp) {
			return tf.ErrorDiagF(err, "Deleting group with object ID: %q", d.Id())
		}
	}

	return nil
}
