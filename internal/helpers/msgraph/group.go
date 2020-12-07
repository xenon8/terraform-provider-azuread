package msgraph

import (
	"context"
	"fmt"
	"strings"

	"github.com/manicminer/hamilton/clients"
	"github.com/manicminer/hamilton/models"
)

type GroupMemberId struct {
	ObjectSubResourceId
	GroupId  string
	MemberId string
}

func GroupMemberIdFrom(groupId, memberId string) GroupMemberId {
	return GroupMemberId{
		ObjectSubResourceId: ObjectSubResourceIdFrom(groupId, "member", memberId),
		GroupId:             groupId,
		MemberId:            memberId,
	}
}

func GroupCheckNameAvailability(ctx context.Context, client *clients.GroupsClient, displayName string, existingId *string) error {
	filter := fmt.Sprintf("displayName eq '%s'", displayName)
	result, _, err := client.List(ctx, filter)
	if err != nil {
		return fmt.Errorf("unable to list existing groups: %+v", err)
	}

	var existing []models.Group
	for _, r := range *result {
		if existingId != nil && *r.ID == *existingId {
			continue
		}
		if strings.EqualFold(displayName, *r.DisplayName) {
			existing = append(existing, r)
		}
	}
	count := len(existing)
	if count > 0 {
		noun := "group was"
		if count > 1 {
			noun = "groups were"
		}
		return fmt.Errorf("`prevent_duplicate_names` was specified and %d existing %s found with display_name %q", count, noun, displayName)
	}
	return nil
}
