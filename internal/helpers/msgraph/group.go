package msgraph

import (
	"context"
	"fmt"
	"strings"

	"github.com/manicminer/hamilton/clients"
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

func GroupCheckNameAvailability(ctx context.Context, client *clients.GroupsClient, displayName string, existingID *string) (*string, error) {
	filter := fmt.Sprintf("displayName eq '%s'", displayName)
	result, _, err := client.List(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("unable to list groups: %+v", err)
	}

	for _, r := range *result {
		if existingID != nil && *existingID == *r.ID {
			continue
		}
		if strings.EqualFold(displayName, *r.DisplayName) {
			return r.ID, nil
		}
	}

	return nil, nil
}
