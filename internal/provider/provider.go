// Copyright (c) The Utilities Provider for Terraform Authors
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/function"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

// Ensure NanoidProvider satisfies various provider interfaces.
var _ provider.Provider = &UtilitiesProvider{}
var _ provider.ProviderWithFunctions = &UtilitiesProvider{}

// UtilitiesProvider defines the provider implementation.
type UtilitiesProvider struct {
	// version is set to the provider version on release, "dev" when the
	// provider is built and ran locally, and "test" when running acceptance
	// testing.
	version string
}

// NanoidProviderModel describes the provider data model.
type NanoidProviderModel struct{}

type UtilitiesProviderData struct{}

func (p *UtilitiesProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "utilities"
	resp.Version = p.version
}

func (p *UtilitiesProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Various utilities for Terraform.",
	}
}

func (p *UtilitiesProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data NanoidProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	providerData := UtilitiesProviderData{}
	resp.DataSourceData = &providerData
	resp.ResourceData = &providerData
}

func (p *UtilitiesProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewFileResource,
		NewNanoIdResource,
	}
}

func (p *UtilitiesProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{}
}

func (p *UtilitiesProvider) Functions(ctx context.Context) []func() function.Function {
	return []func() function.Function{}
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &UtilitiesProvider{
			version: version,
		}
	}
}
