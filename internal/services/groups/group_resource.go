package groups

import (
	"context"
	"fmt"

	"github.com/hashicorp/go-uuid"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"

	"github.com/terraform-providers/terraform-provider-azuread/internal/clients"
	"github.com/terraform-providers/terraform-provider-azuread/internal/tf"
	"github.com/terraform-providers/terraform-provider-azuread/internal/validate"
)

func groupResource() *schema.Resource {
	return &schema.Resource{
		CreateContext: groupResourceCreate,
		ReadContext:   groupResourceRead,
		UpdateContext: groupResourceUpdate,
		DeleteContext: groupResourceDelete,

		Importer: tf.ValidateResourceIDPriorToImport(func(id string) error {
			if _, err := uuid.ParseUUID(id); err != nil {
				return fmt.Errorf("specified ID (%q) is not valid: %s", id, err)
			}
			return nil
		}),

		Schema: map[string]*schema.Schema{
			"name": {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validation.NoZeroValues,
			},

			"description": {
				Type:     schema.TypeString,
				ForceNew: true, // there is no update method available in the SDK
				Optional: true,
			},

			"members": {
				Type:     schema.TypeSet,
				Optional: true,
				Computed: true,
				Set:      schema.HashString,
				Elem: &schema.Schema{
					Type:             schema.TypeString,
					ValidateDiagFunc: validate.UUID,
				},
			},

			"owners": {
				Type:     schema.TypeSet,
				Optional: true,
				Computed: true,
				Set:      schema.HashString,
				Elem: &schema.Schema{
					Type:             schema.TypeString,
					ValidateDiagFunc: validate.UUID,
				},
			},

			"object_id": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"prevent_duplicate_names": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
			},
		},
	}
}

func groupResourceCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	if useMsGraph := meta.(*clients.Client).EnableMsGraphBeta; useMsGraph {
		return groupResourceCreateMsGraph(ctx, d, meta)
	}
	return groupResourceCreateAadGraph(ctx, d, meta)
}

func groupResourceRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	if useMsGraph := meta.(*clients.Client).EnableMsGraphBeta; useMsGraph {
		return groupResourceReadMsGraph(ctx, d, meta)
	}
	return groupResourceReadAadGraph(ctx, d, meta)
}

func groupResourceUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	if useMsGraph := meta.(*clients.Client).EnableMsGraphBeta; useMsGraph {
		return groupResourceUpdateMsGraph(ctx, d, meta)
	}
	return groupResourceUpdateAadGraph(ctx, d, meta)
}

func groupResourceDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	if useMsGraph := meta.(*clients.Client).EnableMsGraphBeta; useMsGraph {
		return groupResourceDeleteMsGraph(ctx, d, meta)
	}
	return groupResourceDeleteAadGraph(ctx, d, meta)
}
