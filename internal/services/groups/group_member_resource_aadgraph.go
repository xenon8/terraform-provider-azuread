package groups

import (
	"context"
	"fmt"
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
		return tf.ErrorDiag(fmt.Sprintf("Listing existing members for group with object ID: %q", id.GroupId), err.Error(), "")
	}
	if len(existingMembers) > 0 {
		for _, v := range existingMembers {
			if strings.EqualFold(v, memberID) {
				return tf.ImportAsExistsDiag("azuread_group_member", id.String())
			}
		}
	}

	if err := aadgraph.GroupAddMember(ctx, client, groupID, memberID); err != nil {
		return tf.ErrorDiag("Adding group member", err.Error(), "")
	}

	d.SetId(id.String())
	return groupMemberResourceRead(ctx, d, meta)
}

func groupMemberResourceReadAadGraph(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*clients.Client).Groups.AadClient

	id, err := aadgraph.ParseGroupMemberId(d.Id())
	if err != nil {
		return tf.ErrorDiag(fmt.Sprintf("Parsing Group Member ID %q", d.Id()), err.Error(), "id")
	}

	members, err := aadgraph.GroupAllMembers(ctx, client, id.GroupId)
	if err != nil {
		return tf.ErrorDiag(fmt.Sprintf("Retrieving members for group with object ID: %q", id.GroupId), err.Error(), "")
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

	if err := d.Set("group_object_id", id.GroupId); err != nil {
		return tf.ErrorDiag("Could not set attribute", err.Error(), "group_object_id")
	}

	if err := d.Set("member_object_id", memberObjectID); err != nil {
		return tf.ErrorDiag("Could not set attribute", err.Error(), "member_object_id")
	}

	return nil
}

func groupMemberResourceDeleteAadGraph(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*clients.Client).Groups.AadClient

	id, err := aadgraph.ParseGroupMemberId(d.Id())
	if err != nil {
		return tf.ErrorDiag(fmt.Sprintf("Parsing Group Member ID %q", d.Id()), err.Error(), "id")
	}

	tf.LockByName(groupMemberResourceName, id.GroupId)
	defer tf.UnlockByName(groupMemberResourceName, id.GroupId)

	if err := aadgraph.GroupRemoveMember(ctx, client, d.Timeout(schema.TimeoutDelete), id.GroupId, id.MemberId); err != nil {
		return tf.ErrorDiag(fmt.Sprintf("Removing member %q from group with object ID: %q", id.MemberId, id.GroupId), err.Error(), "")
	}

	if _, err := aadgraph.WaitForListRemove(ctx, id.MemberId, func() ([]string, error) {
		return aadgraph.GroupAllMembers(ctx, client, id.GroupId)
	}); err != nil {
		return tf.ErrorDiag("Waiting for group membership removal", err.Error(), "")
	}

	return nil
}
