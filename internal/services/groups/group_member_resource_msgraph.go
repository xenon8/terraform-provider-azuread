package groups

import (
	"context"
	"fmt"
	"github.com/terraform-providers/terraform-provider-azuread/internal/helpers/msgraph"
	"log"
	"net/http"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

	"github.com/terraform-providers/terraform-provider-azuread/internal/clients"
	"github.com/terraform-providers/terraform-provider-azuread/internal/helpers/aadgraph"
	"github.com/terraform-providers/terraform-provider-azuread/internal/tf"
)

func groupMemberResourceCreateMsGraph(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*clients.Client).Groups.MsClient

	groupId := d.Get("group_object_id").(string)
	memberId := d.Get("member_object_id").(string)

	id := msgraph.GroupMemberIdFrom(groupId, memberId)

	tf.LockByName(groupMemberResourceName, groupId)
	defer tf.UnlockByName(groupMemberResourceName, groupId)

	group, status, err := client.Get(ctx, groupId)
	if err != nil {
		if status == http.StatusNotFound {
			return tf.ErrorDiag(fmt.Sprintf("Group with object ID %q was not found", groupId), "", "group_id")
		}
		return tf.ErrorDiag(fmt.Sprintf("Retrieving group with object ID: %q", groupId), err.Error(), "")
	}

	existingMembers, _, err := client.ListMembers(ctx, id.GroupId)
	if err != nil {
		return tf.ErrorDiag(fmt.Sprintf("Listing existing members for group with object ID: %q", id.GroupId), err.Error(), "")
	}
	if existingMembers != nil {
		for _, v := range *existingMembers {
			if strings.EqualFold(v, memberId) {
				return tf.ImportAsExistsDiag("azuread_group_member", id.String())
			}
		}
	}

	group.AppendMember(client.BaseClient.Endpoint, client.BaseClient.ApiVersion, memberId)

	if _, err := client.AddMembers(ctx, group); err != nil {
		return tf.ErrorDiag(fmt.Sprintf("Adding group member %q to group %q", memberId, groupId), err.Error(), "")
	}

	d.SetId(id.String())
	return groupMemberResourceRead(ctx, d, meta)
}

func groupMemberResourceReadMsGraph(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*clients.Client).Groups.MsClient

	id, err := aadgraph.ParseGroupMemberId(d.Id())
	if err != nil {
		return tf.ErrorDiag(fmt.Sprintf("Parsing Group Member ID %q", d.Id()), err.Error(), "id")
	}

	members, _, err := client.ListMembers(ctx, id.GroupId)
	if err != nil {
		return tf.ErrorDiag(fmt.Sprintf("Retrieving members for group with object ID: %q", id.GroupId), err.Error(), "")
	}

	var memberObjectId string
	if members != nil {
		for _, objectId := range *members {
			if strings.EqualFold(objectId, id.MemberId) {
				memberObjectId = objectId
				break
			}
		}
	}

	if memberObjectId == "" {
		log.Printf("[DEBUG] Member with ID %q was not found in Group %q - removing from state", id.MemberId, id.GroupId)
		d.SetId("")
		return nil
	}

	if err := d.Set("group_object_id", id.GroupId); err != nil {
		return tf.ErrorDiag("Could not set attribute", err.Error(), "group_object_id")
	}

	if err := d.Set("member_object_id", memberObjectId); err != nil {
		return tf.ErrorDiag("Could not set attribute", err.Error(), "member_object_id")
	}

	return nil
}

func groupMemberResourceDeleteMsGraph(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*clients.Client).Groups.MsClient

	id, err := aadgraph.ParseGroupMemberId(d.Id())
	if err != nil {
		return tf.ErrorDiag(fmt.Sprintf("Parsing Group Member ID %q", d.Id()), err.Error(), "id")
	}

	tf.LockByName(groupMemberResourceName, id.GroupId)
	defer tf.UnlockByName(groupMemberResourceName, id.GroupId)

	if _, err := client.RemoveMembers(ctx, id.GroupId, &[]string{id.MemberId}); err != nil {
		return tf.ErrorDiag(fmt.Sprintf("Removing member %q from group with object ID: %q", id.MemberId, id.GroupId), err.Error(), "")
	}

	if _, err := msgraph.WaitForListRemove(ctx, id.MemberId, func() ([]string, error) {
		members, _, err := client.ListMembers(ctx, id.GroupId)
		if members == nil {
			return make([]string, 0), err
		}
		return *members, err
	}); err != nil {
		return tf.ErrorDiag("Waiting for group membership removal", err.Error(), "")
	}

	return nil
}
