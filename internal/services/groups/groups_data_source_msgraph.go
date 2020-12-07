package groups

import (
	"context"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/manicminer/hamilton/models"

	"github.com/terraform-providers/terraform-provider-azuread/internal/clients"
	"github.com/terraform-providers/terraform-provider-azuread/internal/tf"
)

func groupsDataSourceReadMsGraph(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*clients.Client).Groups.MsClient

	var groups []models.Group
	var expectedCount int

	if displayNames, ok := d.Get("display_names").([]interface{}); ok && len(displayNames) > 0 {
		expectedCount = len(displayNames)
		for _, v := range displayNames {
			displayName := v.(string)
			filter := fmt.Sprintf("displayName eq '%s'", displayName)
			result, _, err := client.List(ctx, filter)
			if err != nil {
				return tf.ErrorDiag(fmt.Sprintf("No group found with display name: %q", v), err.Error(), "name")
			}

			count := len(*result)
			if count > 1 {
				return tf.ErrorDiag(fmt.Sprintf("More than one group found with display name: %q", displayName), err.Error(), "name")
			} else if count == 0 {
				return tf.ErrorDiag(fmt.Sprintf("No group found with display name: %q", v), err.Error(), "name")
			}

			groups = append(groups, (*result)[0])
		}
	} else if objectIds, ok := d.Get("object_ids").([]interface{}); ok && len(objectIds) > 0 {
		expectedCount = len(objectIds)
		for _, v := range objectIds {
			objectId := v.(string)
			group, status, err := client.Get(ctx, objectId)
			if err != nil {
				if status == http.StatusNotFound {
					return tf.ErrorDiag(fmt.Sprintf("No group found with object ID: %q", objectId), "", "object_id")
				}
				return tf.ErrorDiag(fmt.Sprintf("Retrieving group with object ID: %q", objectId), err.Error(), "")
			}

			groups = append(groups, *group)
		}
	}

	if len(groups) != expectedCount {
		return tf.ErrorDiag("Unexpected number of groups returned", fmt.Sprintf("Expected: %d, Actual: %d", expectedCount, len(groups)), "")
	}

	displayNames := make([]string, 0, len(groups))
	objectIds := make([]string, 0, len(groups))
	for _, group := range groups {
		if group.ID == nil || group.DisplayName == nil {
			return tf.ErrorDiag("Bad API response", "API returned group with nil object ID", "")
		}

		objectIds = append(objectIds, *group.ID)
		displayNames = append(displayNames, *group.DisplayName)
	}

	h := sha1.New()
	if _, err := h.Write([]byte(strings.Join(displayNames, "-"))); err != nil {
		return tf.ErrorDiag("Able to compute hash for names", err.Error(), "")
	}

	d.SetId("groups#" + base64.URLEncoding.EncodeToString(h.Sum(nil)))

	if err := d.Set("object_ids", objectIds); err != nil {
		return tf.ErrorDiag("Could not set attribute", err.Error(), "object_ids")
	}

	if err := d.Set("names", displayNames); err != nil {
		return tf.ErrorDiag("Could not set attribute", err.Error(), "names")
	}

	return nil
}
