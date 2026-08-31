package portal

import "fmt"

// ValidatePublicDataPK validates the portal's numeric dataset identifier before
// it is interpolated into a path or query. MCP inputs are untrusted and may skip
// catalog_search, so downstream operations must enforce this boundary too.
func ValidatePublicDataPK(pk string) error {
	if pk == "" || len(pk) > 20 {
		return fmt.Errorf("publicDataPk는 1~20자리 숫자여야 합니다")
	}
	for _, r := range pk {
		if r < '0' || r > '9' {
			return fmt.Errorf("publicDataPk는 숫자만 포함해야 합니다")
		}
	}
	return nil
}
