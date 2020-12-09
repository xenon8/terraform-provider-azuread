package groups

import (
	"context"
	"crypto/sha1"
	"encoding/base64"
	"errors"
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
				return tf.ErrorDiagPathF(err, "name", "No group found with display name: %q", v)
			}

			count := len(*result)
			if count > 1 {
				return tf.ErrorDiagPathF(err, "name", "More than one group found with display name: %q", displayName)
			} else if count == 0 {
				return tf.ErrorDiagPathF(err, "name", "No group found with display name: %q", v)
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
					return tf.ErrorDiagPathF(err, "object_id", "No group found with object ID: %q", objectId)
				}
				return tf.ErrorDiagPathF(err, "object_id", "Retrieving group with object ID: %q", objectId)
			}

			groups = append(groups, *group)
		}
	}

	if len(groups) != expectedCount {
		return tf.ErrorDiagF(fmt.Errorf("Expected: %d, Actual: %d", expectedCount, len(groups)), "Unexpected number of groups returned")
	}

	displayNames := make([]string, 0, len(groups))
	objectIds := make([]string, 0, len(groups))
	for _, group := range groups {
		if group.ID == nil || group.DisplayName == nil {
			return tf.ErrorDiagF(errors.New("API returned group with nil object ID"), "Bad API response")
		}

		objectIds = append(objectIds, *group.ID)
		displayNames = append(displayNames, *group.DisplayName)
	}

	h := sha1.New()
	if _, err := h.Write([]byte(strings.Join(displayNames, "-"))); err != nil {
		return tf.ErrorDiagF(err, "Unable to compute hash for names")
	}

	d.SetId("groups#" + base64.URLEncoding.EncodeToString(h.Sum(nil)))

	if dg := tf.Set(d, "object_ids", objectIds); dg != nil {
		return dg
	}

	if dg := tf.Set(d, "names", displayNames); dg != nil {
		return dg
	}

	return nil
}
