package groups

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

	"github.com/terraform-providers/terraform-provider-azuread/internal/clients"
	"github.com/terraform-providers/terraform-provider-azuread/internal/validate"
)

func groupsData() *schema.Resource {
	return &schema.Resource{
		ReadContext: groupsDataSourceRead,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Schema: map[string]*schema.Schema{
			"object_ids": {
				Type:         schema.TypeList,
				Optional:     true,
				Computed:     true,
				ExactlyOneOf: []string{"names", "object_ids"},
				Elem: &schema.Schema{
					Type:             schema.TypeString,
					ValidateDiagFunc: validate.UUID,
				},
			},

			"names": {
				Type:         schema.TypeList,
				Optional:     true,
				Computed:     true,
				ExactlyOneOf: []string{"names", "object_ids"},
				Elem: &schema.Schema{
					Type:             schema.TypeString,
					ValidateDiagFunc: validate.NoEmptyStrings,
				},
			},
		},
	}
}

func groupsDataSourceRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	if useMsGraph := meta.(*clients.Client).EnableMsGraphBeta; useMsGraph {
		return groupsDataSourceReadMsGraph(ctx, d, meta)
	}
	return groupsDataSourceReadAadGraph(ctx, d, meta)
}
