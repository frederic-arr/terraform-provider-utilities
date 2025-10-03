// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package http_test

import (
	"terraform-provider-utilities/internal/provider"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

//nolint:unparam
func protoV6ProviderFactories() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"utilities": providerserver.NewProtocol6WithError(provider.New("test")()),
	}
}
