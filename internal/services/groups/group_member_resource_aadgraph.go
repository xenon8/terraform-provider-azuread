package groups

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

	"github.com/terraform-providers/terraform-provider-azuread/internal/clients"
	"github.com/terraform-providers/terraform-provider-azuread/internal/helpers/aadgraph"
	"github.com/terraform-providers/terraform-provider-azuread/internal/tf"
)

func groupMemberResourceCreateAadGraph(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*clients.Client).Groups.AadClient

	groupID := d.Get("group_object_id").(string)
	memberID := d.Get("member_object_id").(string)

	id := aadgraph.GroupMemberIdFrom(groupID, memberID)

	tf.LockByName(groupMemberResourceName, groupID)
	defer tf.UnlockByName(groupMemberResourceName, groupID)

	existingMembers, err := aadgraph.GroupAllMembers(ctx, client, groupID)
	if err != nil {
		return tf.ErrorDiagF(err, "Listing existing members for group with object ID: %q", id.GroupId)
	}
	if len(existingMembers) > 0 {
		for _, v := range existingMembers {
			if strings.EqualFold(v, memberID) {
				return tf.ImportAsExistsDiag("azuread_group_member", id.String())
			}
		}
	}

	if err := aadgraph.GroupAddMember(ctx, client, groupID, memberID); err != nil {
		return tf.ErrorDiagF(err, "Adding group member")
	}

	d.SetId(id.String())

	return groupMemberResourceRead(ctx, d, meta)
}

func groupMemberResourceReadAadGraph(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*clients.Client).Groups.AadClient

	id, err := aadgraph.ParseGroupMemberId(d.Id())
	if err != nil {
		return tf.ErrorDiagPathF(err, "id", "Parsing Group Member ID %q", d.Id())
	}

	members, err := aadgraph.GroupAllMembers(ctx, client, id.GroupId)
	if err != nil {
		return tf.ErrorDiagF(err, "Retrieving members for group with object ID: %q", id.GroupId)
	}

	var memberObjectID string
	for _, objectID := range members {
		if strings.EqualFold(objectID, id.MemberId) {
			memberObjectID = objectID
			break
		}
	}

	if memberObjectID == "" {
		d.SetId("")
		return nil
	}

	if dg := tf.Set(d, "group_object_id", id.GroupId); dg != nil {
		return dg
	}

	if dg := tf.Set(d, "member_object_id", memberObjectID); dg != nil {
		return dg
	}

	return nil
}

func groupMemberResourceDeleteAadGraph(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*clients.Client).Groups.AadClient

	id, err := aadgraph.ParseGroupMemberId(d.Id())
	if err != nil {
		return tf.ErrorDiagPathF(err, "id", "Parsing Group Member ID %q", d.Id())
	}

	tf.LockByName(groupMemberResourceName, id.GroupId)
	defer tf.UnlockByName(groupMemberResourceName, id.GroupId)

	if err := aadgraph.GroupRemoveMember(ctx, client, d.Timeout(schema.TimeoutDelete), id.GroupId, id.MemberId); err != nil {
		return tf.ErrorDiagF(err, "Removing member %q from group with object ID: %q", id.MemberId, id.GroupId)
	}

	if _, err := aadgraph.WaitForListRemove(ctx, id.MemberId, func() ([]string, error) {
		return aadgraph.GroupAllMembers(ctx, client, id.GroupId)
	}); err != nil {
		return tf.ErrorDiagF(err, "Waiting for group membership removal")
	}

	return nil
}
