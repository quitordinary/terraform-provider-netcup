// Package provider is the OpenTofu provider for netcup's SCP (ADR-0066).
//
// Today it has a configuration and no resources: the spike (Infrastructure#68)
// proves that OpenTofu installs it from GHCR. Data sources follow in #71,
// resources in #72.
package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Address is the source address users declare. The hostname serves no registry,
// so without the CLI config's oci_mirror `tofu init` fails instead of installing
// a look-alike (ADR-0066).
const Address = "tofu.quitordinary.com/quitordinary/netcup"

const envRefreshToken = "NETCUP_SCP_REFRESH_TOKEN" //nolint:gosec // G101: an environment variable's name, not a credential

// New returns a constructor for the provider at the given version.
func New(version string) func() provider.Provider {
	return func() provider.Provider { return &netcupProvider{version: version} }
}

type netcupProvider struct {
	version string
}

type providerModel struct {
	RefreshToken types.String `tfsdk:"refresh_token"`
}

func (p *netcupProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "netcup"
	resp.Version = p.version
}

func (p *netcupProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "netcup's Server Control Panel (SCP REST API).",
		Attributes: map[string]schema.Attribute{
			"refresh_token": schema.StringAttribute{
				Description: "Offline refresh token from `netcupctl login`. Defaults to " + envRefreshToken + ".",
				Optional:    true,
				Sensitive:   true,
			},
		},
	}
}

func (p *netcupProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// Nothing reads the token yet; with no resources, a missing one is not an error.
	_ = refreshToken(config)
}

func refreshToken(config providerModel) string {
	if !config.RefreshToken.IsNull() && !config.RefreshToken.IsUnknown() {
		return config.RefreshToken.ValueString()
	}
	return os.Getenv(envRefreshToken)
}

func (p *netcupProvider) Resources(context.Context) []func() resource.Resource {
	return nil
}

func (p *netcupProvider) DataSources(context.Context) []func() datasource.DataSource {
	return nil
}
