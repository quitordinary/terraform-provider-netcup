package provider

import (
	"context"
	"testing"

	fwprovider "github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestSchemaMarksTheTokenSensitive(t *testing.T) {
	var resp fwprovider.SchemaResponse
	New("test")().Schema(context.Background(), fwprovider.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema: %v", resp.Diagnostics)
	}
	attr, ok := resp.Schema.Attributes["refresh_token"]
	if !ok || !attr.IsSensitive() || !attr.IsOptional() {
		t.Fatalf("refresh_token must be optional and sensitive: %#v", attr)
	}
	if diags := resp.Schema.ValidateImplementation(context.Background()); diags.HasError() {
		t.Fatalf("schema implementation: %v", diags)
	}
}

func TestRefreshTokenPrefersConfigOverEnvironment(t *testing.T) {
	t.Setenv(envRefreshToken, "from-env")
	if got := refreshToken(providerModel{RefreshToken: types.StringValue("from-config")}); got != "from-config" {
		t.Errorf("got %q", got)
	}
	if got := refreshToken(providerModel{RefreshToken: types.StringNull()}); got != "from-env" {
		t.Errorf("got %q", got)
	}
}
