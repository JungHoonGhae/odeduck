package main

import (
	"strings"
	"testing"
)

func TestRootCommandPresentsOpenDataCTLBrand(t *testing.T) {
	if rootCmd.Use != "opendatactl" {
		t.Fatalf("root command use = %q", rootCmd.Use)
	}
	if !strings.Contains(rootCmd.Long, "OpenDataCTL") {
		t.Fatalf("root command does not present the product brand: %q", rootCmd.Long)
	}
}
