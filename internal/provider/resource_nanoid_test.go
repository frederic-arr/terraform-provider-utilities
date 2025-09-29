// Copyright (c) The Utilities Provider for Terraform Authors
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func testCheckLen(expectedLen int) func(input string) error {
	return func(input string) error {
		if len(input) != expectedLen {
			return fmt.Errorf("expected length %d, actual length %d", expectedLen, len(input))
		}

		return nil
	}
}

func TestAccIdResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccIdResourceConfigEmpty(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("utilities_nanoid.test", "length", "21"),
					resource.TestCheckResourceAttr("utilities_nanoid.test", "alphabet", "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ_abcdefghijklmnopqrstuvwxyz-"),
					resource.TestCheckResourceAttrWith("utilities_nanoid.test", "id", testCheckLen(21)),
				),
			},
			{
				ResourceName:      "utilities_nanoid.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccIdResource_WithLength(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccIdResourceConfig(11, nil),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("utilities_nanoid.test", "length", "11"),
					resource.TestCheckResourceAttr("utilities_nanoid.test", "alphabet", "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ_abcdefghijklmnopqrstuvwxyz-"),
					resource.TestCheckResourceAttrWith("utilities_nanoid.test", "id", testCheckLen(11)),
				),
			},
			{
				ResourceName:      "utilities_nanoid.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccIdResourceConfig(length int, alphabet *string) string {
	lengthStr := fmt.Sprintf("length = %d", length)
	alphabetStr := ""
	if alphabet != nil {
		alphabetStr = fmt.Sprintf("alphabet = %q", *alphabet)
	}
	return fmt.Sprintf(`
resource "utilities_nanoid" "test" {
  %s
  %s
}
`, lengthStr, alphabetStr)
}

func testAccIdResourceConfigEmpty() string {
	return `resource "utilities_nanoid" "test" {}`
}
