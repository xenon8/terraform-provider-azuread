package applications

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/Azure/azure-sdk-for-go/services/graphrbac/1.6/graphrbac"
	"github.com/hashicorp/go-uuid"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"

	"github.com/terraform-providers/terraform-provider-azuread/internal/clients"
	"github.com/terraform-providers/terraform-provider-azuread/internal/helpers/aadgraph"
	"github.com/terraform-providers/terraform-provider-azuread/internal/tf"
	"github.com/terraform-providers/terraform-provider-azuread/internal/utils"
	"github.com/terraform-providers/terraform-provider-azuread/internal/validate"
)

const resourceApplicationName = "azuread_application"

func applicationResource() *schema.Resource {
	return &schema.Resource{
		CreateContext: applicationResourceCreate,
		ReadContext:   applicationResourceRead,
		UpdateContext: applicationResourceUpdate,
		DeleteContext: applicationResourceDelete,

		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(5 * time.Minute),
			Read:   schema.DefaultTimeout(5 * time.Minute),
			Update: schema.DefaultTimeout(5 * time.Minute),
			Delete: schema.DefaultTimeout(5 * time.Minute),
		},

		Importer: tf.ValidateResourceIDPriorToImport(func(id string) error {
			if _, err := uuid.ParseUUID(id); err != nil {
				return fmt.Errorf("specified ID (%q) is not valid: %s", id, err)
			}
			return nil
		}),

		Schema: map[string]*schema.Schema{
			"name": {
				Type:             schema.TypeString,
				Required:         true,
				ValidateDiagFunc: validate.NoEmptyStrings,
			},

			"available_to_other_tenants": {
				Type:     schema.TypeBool,
				Optional: true,
			},

			"group_membership_claims": {
				Type:     schema.TypeString,
				Optional: true,
				ValidateFunc: validation.StringInSlice([]string{
					string(graphrbac.All),
					string(graphrbac.None),
					string(graphrbac.SecurityGroup),
					"DirectoryRole",    // missing from sdk: https://github.com/Azure/azure-sdk-for-go/issues/7857
					"ApplicationGroup", //missing from sdk:https://github.com/Azure/azure-sdk-for-go/issues/8244
				}, false),
			},

			"homepage": {
				Type:             schema.TypeString,
				Optional:         true,
				Computed:         true,
				ValidateDiagFunc: validate.URLIsHTTPOrHTTPS,
			},

			"identifier_uris": {
				Type:     schema.TypeList,
				Optional: true,
				Computed: true,
				Elem: &schema.Schema{
					Type:             schema.TypeString,
					ValidateDiagFunc: validate.URLIsAppURI,
				},
			},

			"logout_url": {
				Type:             schema.TypeString,
				Optional:         true,
				ValidateDiagFunc: validate.URLIsHTTPOrHTTPS,
			},

			"oauth2_allow_implicit_flow": {
				Type:     schema.TypeBool,
				Optional: true,
			},

			"public_client": {
				Type:     schema.TypeBool,
				Optional: true,
				Computed: true,
			},

			"reply_urls": {
				Type:     schema.TypeSet,
				Optional: true,
				Computed: true,
				Elem: &schema.Schema{
					Type:             schema.TypeString,
					ValidateDiagFunc: validate.NoEmptyStrings,
				},
			},

			"type": {
				Type:         schema.TypeString,
				Optional:     true,
				ValidateFunc: validation.StringInSlice([]string{"webapp/api", "native"}, false),
				Default:      "webapp/api",
			},

			"app_role": {
				Type:       schema.TypeSet,
				Optional:   true,
				Computed:   true,
				ConfigMode: schema.SchemaConfigModeAttr,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"id": {
							Type:     schema.TypeString,
							Computed: true,
						},

						"allowed_member_types": {
							Type:     schema.TypeSet,
							Required: true,
							MinItems: 1,
							Elem: &schema.Schema{
								Type: schema.TypeString,
								ValidateFunc: validation.StringInSlice(
									[]string{"User", "Application"},
									false,
								),
							},
						},

						"description": {
							Type:             schema.TypeString,
							Required:         true,
							ValidateDiagFunc: validate.NoEmptyStrings,
						},

						"display_name": {
							Type:             schema.TypeString,
							Required:         true,
							ValidateDiagFunc: validate.NoEmptyStrings,
						},

						"is_enabled": {
							Type:     schema.TypeBool,
							Optional: true,
							Default:  true,
						},

						"value": {
							Type:     schema.TypeString,
							Optional: true,
							Computed: true,
						},
					},
				},
			},

			"oauth2_permissions": {
				Type:       schema.TypeSet,
				Optional:   true,
				Computed:   true,
				ConfigMode: schema.SchemaConfigModeAttr,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"admin_consent_description": {
							Type:             schema.TypeString,
							Optional:         true,
							Computed:         true,
							ValidateDiagFunc: validate.NoEmptyStrings,
						},

						"admin_consent_display_name": {
							Type:             schema.TypeString,
							Optional:         true,
							Computed:         true,
							ValidateDiagFunc: validate.NoEmptyStrings,
						},

						"id": {
							Type:     schema.TypeString,
							Computed: true,
						},

						"is_enabled": {
							Type:     schema.TypeBool,
							Optional: true,
							Computed: true,
						},

						"type": {
							Type:         schema.TypeString,
							Optional:     true,
							Computed:     true,
							ValidateFunc: validation.StringInSlice([]string{"Admin", "User"}, false),
						},

						"user_consent_description": {
							Type:     schema.TypeString,
							Optional: true,
							Computed: true,
						},

						"user_consent_display_name": {
							Type:     schema.TypeString,
							Optional: true,
							Computed: true,
						},

						"value": {
							Type:             schema.TypeString,
							Optional:         true,
							Computed:         true,
							ValidateDiagFunc: validate.NoEmptyStrings,
						},
					},
				},
			},

			"optional_claims": {
				Type:     schema.TypeList,
				Optional: true,
				MaxItems: 1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"access_token": aadgraph.SchemaOptionalClaims(),
						"id_token":     aadgraph.SchemaOptionalClaims(),
						// TODO: enable when https://github.com/Azure/azure-sdk-for-go/issues/9714 resolved
						//"saml_token": aadgraph.SchemaOptionalClaims(),
					},
				},
			},

			"required_resource_access": {
				Type:     schema.TypeSet,
				Optional: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"resource_app_id": {
							Type:     schema.TypeString,
							Required: true,
						},

						"resource_access": {
							Type:     schema.TypeList,
							Required: true,
							Elem: &schema.Resource{
								Schema: map[string]*schema.Schema{
									"id": {
										Type:             schema.TypeString,
										Required:         true,
										ValidateDiagFunc: validate.UUID,
									},

									"type": {
										Type:     schema.TypeString,
										Required: true,
										ValidateFunc: validation.StringInSlice(
											[]string{"Scope", "Role"},
											false, // force case sensitivity
										),
									},
								},
							},
						},
					},
				},
			},

			"owners": {
				Type:     schema.TypeSet,
				Optional: true,
				Computed: true,
				Elem: &schema.Schema{
					Type:             schema.TypeString,
					ValidateDiagFunc: validate.NoEmptyStrings,
				},
			},

			"application_id": {
				Type:     schema.TypeString,
				Computed: true,
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

func applicationResourceCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*clients.Client).Applications.ApplicationsClient

	name := d.Get("name").(string)

	if d.Get("prevent_duplicate_names").(bool) {
		existingApp, err := aadgraph.ApplicationFindByName(ctx, client, name)
		if err != nil {
			return tf.ErrorDiagPathF(err, "name", "Could not check for existing application(s)")
		}
		if existingApp != nil {
			if existingApp.ObjectID == nil {
				return tf.ImportAsDuplicateDiag("azuread_application", "unknown", name)
			}
			return tf.ImportAsDuplicateDiag("azuread_application", *existingApp.ObjectID, name)
		}
	}

	if err := applicationValidateRolesScopes(d.Get("app_role").(*schema.Set).List(), d.Get("oauth2_permissions").(*schema.Set).List()); err != nil {
		return tf.ErrorDiagPathF(err, "app_role", "Checking for duplicate app role / oauth2_permissions values")
	}

	appType := d.Get("type")
	identUrls, hasIdentUrls := d.GetOk("identifier_uris")
	if appType == "native" {
		if hasIdentUrls {
			return tf.ErrorDiagPathF(nil, "identifier_uris", "Property is not required for a native application")
		}
	}

	// We don't send Oauth2Permissions here because applications tend to get a default `user_impersonation` scope
	// defined, which will either conflict if we also define it, or create an unwanted diff if we don't
	// After creating the application, we update it later before this function returns, including any Oauth2Permissions
	properties := graphrbac.ApplicationCreateParameters{
		DisplayName:             &name,
		IdentifierUris:          tf.ExpandStringSlicePtr(identUrls.([]interface{})),
		ReplyUrls:               tf.ExpandStringSlicePtr(d.Get("reply_urls").(*schema.Set).List()),
		AvailableToOtherTenants: utils.Bool(d.Get("available_to_other_tenants").(bool)),
		RequiredResourceAccess:  expandApplicationRequiredResourceAccess(d),
		OptionalClaims:          expandApplicationOptionalClaims(d),
	}

	if v, ok := d.GetOk("homepage"); ok {
		properties.Homepage = utils.String(v.(string))
	}

	if v, ok := d.GetOk("logout_url"); ok {
		properties.LogoutURL = utils.String(v.(string))
	}

	if v, ok := d.GetOk("oauth2_allow_implicit_flow"); ok {
		properties.Oauth2AllowImplicitFlow = utils.Bool(v.(bool))
	}

	if v, ok := d.GetOk("public_client"); ok {
		properties.PublicClient = utils.Bool(v.(bool))
	}

	if v, ok := d.GetOk("group_membership_claims"); ok {
		properties.GroupMembershipClaims = graphrbac.GroupMembershipClaimTypes(v.(string))
	}

	app, err := client.Create(ctx, properties)
	if err != nil {
		return tf.ErrorDiagF(err, "Could not create application")
	}
	if app.ObjectID == nil || *app.ObjectID == "" {
		return tf.ErrorDiagF(errors.New("Bad API response"), "Object ID returned for application is nil/empty")
	}

	d.SetId(*app.ObjectID)

	_, err = aadgraph.WaitForCreationReplication(ctx, d.Timeout(schema.TimeoutCreate), func() (interface{}, error) {
		return client.Get(ctx, *app.ObjectID)
	})
	if err != nil {
		return tf.ErrorDiagF(err, "Waiting for Application with object ID: %q", *app.ObjectID)
	}

	// follow suggested hack for azure-cli
	// AAD aadgraph doesn't have the API to create a native app, aka public client, the recommended hack is
	// to create a web app first, then convert to a native one
	if appType == "native" {
		properties := graphrbac.ApplicationUpdateParameters{
			Homepage:       nil,
			IdentifierUris: &[]string{},
			PublicClient:   utils.Bool(true),
		}
		if _, err := client.Patch(ctx, *app.ObjectID, properties); err != nil {
			return tf.ErrorDiagF(err, "Updating Application with object ID: %q", *app.ObjectID)
		}
	}

	if v, ok := d.GetOk("app_role"); ok {
		appRoles := expandApplicationAppRoles(v)
		if appRoles != nil {
			if err := aadgraph.AppRolesSet(ctx, client, *app.ObjectID, appRoles); err != nil {
				return tf.ErrorDiagPathF(err, "app_role", "Could not set App Roles")
			}
		}
	}

	if v, ok := d.GetOk("oauth2_permissions"); ok {
		oauth2Permissions := expandApplicationOAuth2Permissions(v)
		if oauth2Permissions != nil {
			if err := aadgraph.OAuth2PermissionsSet(ctx, client, *app.ObjectID, oauth2Permissions); err != nil {
				return tf.ErrorDiagPathF(err, "oauth2_permissions", "Could not set OAuth2 Permissions")
			}
		}
	}

	if v, ok := d.GetOk("owners"); ok {
		desiredOwners := *tf.ExpandStringSlicePtr(v.(*schema.Set).List())
		if err := applicationSetOwnersTo(ctx, client, *app.ObjectID, desiredOwners); err != nil {
			return tf.ErrorDiagPathF(err, "owners", "Could not set Owners")
		}
	}

	return applicationResourceRead(ctx, d, meta)
}

func applicationResourceUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*clients.Client).Applications.ApplicationsClient

	name := d.Get("name").(string)

	if d.HasChange("name") && d.Get("prevent_duplicate_names").(bool) {
		existingApp, err := aadgraph.ApplicationFindByName(ctx, client, name)
		if err != nil {
			return tf.ErrorDiagPathF(err, "name", "Could not check for existing application(s)")
		}
		if existingApp != nil {
			if existingApp.ObjectID == nil {
				return tf.ImportAsDuplicateDiag("azuread_application", "unknown", name)
			}
			return tf.ImportAsDuplicateDiag("azuread_application", *existingApp.ObjectID, name)
		}
	}

	if err := applicationValidateRolesScopes(d.Get("app_role").(*schema.Set).List(), d.Get("oauth2_permissions").(*schema.Set).List()); err != nil {
		return tf.ErrorDiagPathF(err, "app_role", "Checking for duplicate app role / oauth2_permissions values")
	}

	var properties graphrbac.ApplicationUpdateParameters

	if d.HasChange("name") {
		properties.DisplayName = &name
	}

	if d.HasChange("homepage") {
		properties.Homepage = utils.String(d.Get("homepage").(string))
	}

	if d.HasChange("logout_url") {
		properties.LogoutURL = utils.String(d.Get("logout_url").(string))
	}

	if d.HasChange("identifier_uris") {
		properties.IdentifierUris = tf.ExpandStringSlicePtr(d.Get("identifier_uris").([]interface{}))
	}

	if d.HasChange("reply_urls") {
		properties.ReplyUrls = tf.ExpandStringSlicePtr(d.Get("reply_urls").(*schema.Set).List())
	}

	if d.HasChange("available_to_other_tenants") {
		properties.AvailableToOtherTenants = utils.Bool(d.Get("available_to_other_tenants").(bool))
	}

	if d.HasChange("oauth2_allow_implicit_flow") {
		properties.Oauth2AllowImplicitFlow = utils.Bool(d.Get("oauth2_allow_implicit_flow").(bool))
	}

	if d.HasChange("public_client") {
		properties.PublicClient = utils.Bool(d.Get("public_client").(bool))
	}

	if d.HasChange("required_resource_access") {
		properties.RequiredResourceAccess = expandApplicationRequiredResourceAccess(d)
	}

	if d.HasChange("optional_claims") {
		properties.OptionalClaims = expandApplicationOptionalClaims(d)
	}

	if d.HasChange("group_membership_claims") {
		properties.GroupMembershipClaims = graphrbac.GroupMembershipClaimTypes(d.Get("group_membership_claims").(string))
	}

	// AAD Graph is only capable of specifying previous-generation public client configurations
	if d.HasChange("type") {
		switch appType := d.Get("type"); appType {
		case "webapp/api":
			properties.PublicClient = utils.Bool(false)
			properties.IdentifierUris = tf.ExpandStringSlicePtr(d.Get("identifier_uris").([]interface{}))
		case "native":
			properties.PublicClient = utils.Bool(true)
			properties.IdentifierUris = &[]string{}
		default:
			return tf.ErrorDiagPathF(fmt.Errorf("Unknown application type %v. Supported types are: webapp/api, native", appType),
				"type", "Updating Application with object ID: %q", d.Id())
		}
	}

	if _, err := client.Patch(ctx, d.Id(), properties); err != nil {
		return tf.ErrorDiagF(err, "Updating Application with object ID %q", d.Id())
	}

	if d.HasChange("app_role") {
		appRoles := expandApplicationAppRoles(d.Get("app_role"))
		if appRoles != nil {
			if err := aadgraph.AppRolesSet(ctx, client, d.Id(), appRoles); err != nil {
				return tf.ErrorDiagPathF(err, "app_role", "Could not set App Roles")
			}
		}
	}

	if d.HasChange("oauth2_permissions") {
		oauth2Permissions := expandApplicationOAuth2Permissions(d.Get("oauth2_permissions"))
		if oauth2Permissions != nil {
			if err := aadgraph.OAuth2PermissionsSet(ctx, client, d.Id(), oauth2Permissions); err != nil {
				return tf.ErrorDiagPathF(err, "oauth2_permissions", "Could not set OAuth2 Permissions")
			}
		}
	}

	if d.HasChange("owners") {
		desiredOwners := *tf.ExpandStringSlicePtr(d.Get("owners").(*schema.Set).List())
		if err := applicationSetOwnersTo(ctx, client, d.Id(), desiredOwners); err != nil {
			return tf.ErrorDiagPathF(err, "owners", "Could not set Owners")
		}
	}

	return applicationResourceRead(ctx, d, meta)
}

func applicationResourceRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*clients.Client).Applications.ApplicationsClient

	app, err := client.Get(ctx, d.Id())
	if err != nil {
		if utils.ResponseWasNotFound(app.Response) {
			log.Printf("[DEBUG] Application with Object ID %q was not found - removing from state", d.Id())
			d.SetId("")
			return nil
		}

		return tf.ErrorDiagPathF(err, "application_object_id", "Retrieving Application with object ID %q", d.Id())
	}

	if dg := tf.Set(d, "object_id", app.ObjectID); dg != nil {
		return dg
	}

	if dg := tf.Set(d, "application_id", app.AppID); dg != nil {
		return dg
	}

	if dg := tf.Set(d, "name", app.DisplayName); dg != nil {
		return dg
	}

	if dg := tf.Set(d, "homepage", app.Homepage); dg != nil {
		return dg
	}

	if dg := tf.Set(d, "logout_url", app.LogoutURL); dg != nil {
		return dg
	}

	if dg := tf.Set(d, "available_to_other_tenants", app.AvailableToOtherTenants); dg != nil {
		return dg
	}

	if dg := tf.Set(d, "oauth2_allow_implicit_flow", app.Oauth2AllowImplicitFlow); dg != nil {
		return dg
	}

	if dg := tf.Set(d, "public_client", app.PublicClient); dg != nil {
		return dg
	}

	var appType string
	if v := app.PublicClient; v != nil && *v {
		appType = "native"
	} else {
		appType = "webapp/api"
	}

	if dg := tf.Set(d, "type", appType); dg != nil {
		return dg
	}

	if dg := tf.Set(d, "group_membership_claims", app.GroupMembershipClaims); dg != nil {
		return dg
	}

	if dg := tf.Set(d, "identifier_uris", tf.FlattenStringSlicePtr(app.IdentifierUris)); dg != nil {
		return dg
	}

	if dg := tf.Set(d, "reply_urls", tf.FlattenStringSlicePtr(app.ReplyUrls)); dg != nil {
		return dg
	}

	if dg := tf.Set(d, "required_resource_access", flattenApplicationRequiredResourceAccess(app.RequiredResourceAccess)); dg != nil {
		return dg
	}

	if dg := tf.Set(d, "optional_claims", flattenApplicationOptionalClaims(app.OptionalClaims)); dg != nil {
		return dg
	}

	if dg := tf.Set(d, "app_role", aadgraph.FlattenAppRoles(app.AppRoles)); dg != nil {
		return dg
	}

	if dg := tf.Set(d, "oauth2_permissions", aadgraph.FlattenOauth2Permissions(app.Oauth2Permissions)); dg != nil {
		return dg
	}

	owners, err := aadgraph.ApplicationAllOwners(ctx, client, d.Id())
	if err != nil {
		return tf.ErrorDiagPathF(err, "owners", "Could not retrieve owners for application with object ID %q", *app.ObjectID)
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

func applicationResourceDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	client := meta.(*clients.Client).Applications.ApplicationsClient

	// in order to delete an application which is available to other tenants, we first have to disable this setting
	availableToOtherTenants := d.Get("available_to_other_tenants").(bool)
	if availableToOtherTenants {
		log.Printf("[DEBUG] Application is available to other tenants - disabling that feature before deleting.")
		properties := graphrbac.ApplicationUpdateParameters{
			AvailableToOtherTenants: utils.Bool(false),
		}

		if _, err := client.Patch(ctx, d.Id(), properties); err != nil {
			return tf.ErrorDiagF(err, "Updating Application with object ID %q", d.Id())
		}
	}

	resp, err := client.Delete(ctx, d.Id())
	if err != nil {
		if !utils.ResponseWasNotFound(resp) {
			return tf.ErrorDiagF(err, "Deleting Application with object ID %q", d.Id())
		}
	}

	return nil
}

func expandApplicationRequiredResourceAccess(d *schema.ResourceData) *[]graphrbac.RequiredResourceAccess {
	requiredResourcesAccesses := d.Get("required_resource_access").(*schema.Set).List()
	result := make([]graphrbac.RequiredResourceAccess, 0)

	for _, raw := range requiredResourcesAccesses {
		requiredResourceAccess := raw.(map[string]interface{})
		resource_app_id := requiredResourceAccess["resource_app_id"].(string)

		result = append(result,
			graphrbac.RequiredResourceAccess{
				ResourceAppID: &resource_app_id,
				ResourceAccess: expandApplicationResourceAccess(
					requiredResourceAccess["resource_access"].([]interface{}),
				),
			},
		)
	}
	return &result
}

func expandApplicationResourceAccess(in []interface{}) *[]graphrbac.ResourceAccess {
	resourceAccesses := make([]graphrbac.ResourceAccess, 0, len(in))
	for _, resourceAccessRaw := range in {
		resourceAccess := resourceAccessRaw.(map[string]interface{})

		resourceId := resourceAccess["id"].(string)
		resourceType := resourceAccess["type"].(string)

		resourceAccesses = append(resourceAccesses,
			graphrbac.ResourceAccess{
				ID:   &resourceId,
				Type: &resourceType,
			},
		)
	}

	return &resourceAccesses
}

func flattenApplicationRequiredResourceAccess(in *[]graphrbac.RequiredResourceAccess) []map[string]interface{} {
	if in == nil {
		return []map[string]interface{}{}
	}

	result := make([]map[string]interface{}, 0, len(*in))
	for _, requiredResourceAccess := range *in {
		resource := make(map[string]interface{})
		if requiredResourceAccess.ResourceAppID != nil {
			resource["resource_app_id"] = *requiredResourceAccess.ResourceAppID
		}

		resource["resource_access"] = flattenApplicationResourceAccess(requiredResourceAccess.ResourceAccess)

		result = append(result, resource)
	}

	return result
}

func flattenApplicationResourceAccess(in *[]graphrbac.ResourceAccess) []interface{} {
	if in == nil {
		return []interface{}{}
	}

	accesses := make([]interface{}, 0)
	for _, resourceAccess := range *in {
		access := make(map[string]interface{})
		if resourceAccess.ID != nil {
			access["id"] = *resourceAccess.ID
		}
		if resourceAccess.Type != nil {
			access["type"] = *resourceAccess.Type
		}
		accesses = append(accesses, access)
	}

	return accesses
}

func expandApplicationOptionalClaims(d *schema.ResourceData) *graphrbac.OptionalClaims {
	result := graphrbac.OptionalClaims{}

	for _, raw := range d.Get("optional_claims").([]interface{}) {
		optionalClaims := raw.(map[string]interface{})
		result.AccessToken = expandApplicationOptionalClaim(optionalClaims["access_token"].([]interface{}))
		result.IDToken = expandApplicationOptionalClaim(optionalClaims["id_token"].([]interface{}))
		// TODO: enable when https://github.com/Azure/azure-sdk-for-go/issues/9714 resolved
		//result.SamlToken = expandApplicationOptionalClaim(optionalClaims["saml_token"].([]interface{}))
	}
	return &result
}

func expandApplicationOptionalClaim(in []interface{}) *[]graphrbac.OptionalClaim {
	optionalClaims := make([]graphrbac.OptionalClaim, 0, len(in))
	for _, optionalClaimRaw := range in {
		optionalClaim := optionalClaimRaw.(map[string]interface{})

		name := optionalClaim["name"].(string)
		essential := optionalClaim["essential"].(bool)
		additionalProps := make([]string, 0)

		if props := optionalClaim["additional_properties"]; props != nil {
			for _, prop := range props.([]interface{}) {
				additionalProps = append(additionalProps, prop.(string))
			}
		}

		newClaim := graphrbac.OptionalClaim{
			Name:                 &name,
			Essential:            &essential,
			AdditionalProperties: &additionalProps,
		}

		if source := optionalClaim["source"].(string); source != "" {
			newClaim.Source = &source
		}

		optionalClaims = append(optionalClaims, newClaim)
	}

	return &optionalClaims
}

func flattenApplicationOptionalClaims(in *graphrbac.OptionalClaims) interface{} {
	var result []map[string]interface{}

	if in == nil {
		return result
	}

	optionalClaims := make(map[string]interface{})
	if claims := flattenApplicationOptionalClaimsList(in.AccessToken); len(claims) > 0 {
		optionalClaims["access_token"] = claims
	}
	if claims := flattenApplicationOptionalClaimsList(in.IDToken); len(claims) > 0 {
		optionalClaims["id_token"] = claims
	}
	// TODO: enable when https://github.com/Azure/azure-sdk-for-go/issues/9714 resolved
	//if claims := flattenApplicationOptionalClaimsList(in.SamlToken); len(claims) > 0 {
	//	optionalClaims["saml_token"] = claims
	//}
	if len(optionalClaims) == 0 {
		return result
	}
	result = append(result, optionalClaims)
	return result
}

func flattenApplicationOptionalClaimsList(in *[]graphrbac.OptionalClaim) []interface{} {
	if in == nil {
		return []interface{}{}
	}

	optionalClaims := make([]interface{}, 0)
	for _, claim := range *in {
		optionalClaim := make(map[string]interface{})
		if claim.Name != nil {
			optionalClaim["name"] = *claim.Name
		}
		if claim.Source != nil {
			optionalClaim["source"] = *claim.Source
		}
		if claim.Essential != nil {
			optionalClaim["essential"] = *claim.Essential
		}
		additionalProperties := make([]string, 0)
		if props := claim.AdditionalProperties; props != nil {
			for _, prop := range props.([]interface{}) {
				additionalProperties = append(additionalProperties, prop.(string))
			}
		}
		optionalClaim["additional_properties"] = additionalProperties
		optionalClaims = append(optionalClaims, optionalClaim)
	}

	return optionalClaims
}

func expandApplicationAppRoles(i interface{}) *[]graphrbac.AppRole {
	input := i.(*schema.Set).List()
	output := make([]graphrbac.AppRole, 0, len(input))

	for _, appRoleRaw := range input {
		appRole := appRoleRaw.(map[string]interface{})

		appRoleID := appRole["id"].(string)
		if appRoleID == "" {
			appRoleID, _ = uuid.GenerateUUID()
		}

		var appRoleAllowedMemberTypes []string
		for _, appRoleAllowedMemberType := range appRole["allowed_member_types"].(*schema.Set).List() {
			appRoleAllowedMemberTypes = append(appRoleAllowedMemberTypes, appRoleAllowedMemberType.(string))
		}

		appRoleDescription := appRole["description"].(string)
		appRoleDisplayName := appRole["display_name"].(string)
		appRoleIsEnabled := appRole["is_enabled"].(bool)

		var appRoleValue *string
		if v, ok := appRole["value"].(string); ok {
			appRoleValue = &v
		}

		output = append(output,
			graphrbac.AppRole{
				ID:                 &appRoleID,
				AllowedMemberTypes: &appRoleAllowedMemberTypes,
				Description:        &appRoleDescription,
				DisplayName:        &appRoleDisplayName,
				IsEnabled:          &appRoleIsEnabled,
				Value:              appRoleValue,
			},
		)
	}

	return &output
}

func expandApplicationOAuth2Permissions(i interface{}) *[]graphrbac.OAuth2Permission {
	input := i.(*schema.Set).List()
	result := make([]graphrbac.OAuth2Permission, 0)

	for _, raw := range input {
		OAuth2Permissions := raw.(map[string]interface{})

		AdminConsentDescription := OAuth2Permissions["admin_consent_description"].(string)
		AdminConsentDisplayName := OAuth2Permissions["admin_consent_display_name"].(string)
		ID := OAuth2Permissions["id"].(string)
		if ID == "" {
			ID, _ = uuid.GenerateUUID()
		}

		IsEnabled := OAuth2Permissions["is_enabled"].(bool)
		Type := OAuth2Permissions["type"].(string)
		UserConsentDescription := OAuth2Permissions["user_consent_description"].(string)
		UserConsentDisplayName := OAuth2Permissions["user_consent_display_name"].(string)
		Value := OAuth2Permissions["value"].(string)

		result = append(result,
			graphrbac.OAuth2Permission{
				AdminConsentDescription: &AdminConsentDescription,
				AdminConsentDisplayName: &AdminConsentDisplayName,
				ID:                      &ID,
				IsEnabled:               &IsEnabled,
				Type:                    &Type,
				UserConsentDescription:  &UserConsentDescription,
				UserConsentDisplayName:  &UserConsentDisplayName,
				Value:                   &Value,
			},
		)
	}
	return &result
}

func applicationSetOwnersTo(ctx context.Context, client *graphrbac.ApplicationsClient, id string, desiredOwners []string) error {
	existingOwners, err := aadgraph.ApplicationAllOwners(ctx, client, id)
	if err != nil {
		return err
	}

	ownersForRemoval := utils.Difference(existingOwners, desiredOwners)
	ownersToAdd := utils.Difference(desiredOwners, existingOwners)

	// add owners first to prevent a possible situation where terraform revokes its own access before adding it back.
	if err := aadgraph.ApplicationAddOwners(ctx, client, id, ownersToAdd); err != nil {
		return err
	}

	for _, ownerToDelete := range ownersForRemoval {
		log.Printf("[DEBUG] Removing owner with id %q from Application with id %q", ownerToDelete, id)
		if resp, err := client.RemoveOwner(ctx, id, ownerToDelete); err != nil {
			if !utils.ResponseWasNotFound(resp) {
				return fmt.Errorf("deleting owner %q from Application with ID %q: %+v", ownerToDelete, id, err)
			}
		}
	}

	return nil
}

func applicationValidateRolesScopes(appRoles, oauth2Permissions []interface{}) error {
	var values []string

	for _, roleRaw := range appRoles {
		role := roleRaw.(map[string]interface{})
		if val := role["value"].(string); val != "" {
			values = append(values, val)
		}
	}

	for _, scopeRaw := range oauth2Permissions {
		scope := scopeRaw.(map[string]interface{})
		if val := scope["value"].(string); val != "" {
			values = append(values, val)
		}
	}

	encountered := make([]string, len(values))
	for _, val := range values {
		for _, en := range encountered {
			if en == val {
				return fmt.Errorf("validation failed: duplicate value found: %q", val)
			}
		}
		encountered = append(encountered, val)
	}

	return nil
}
