package portal

import "testing"

func TestValidatePublicDataPK(t *testing.T) {
	for _, pk := range []string{"1", "15077974", "12345678901234567890"} {
		if err := ValidatePublicDataPK(pk); err != nil {
			t.Errorf("valid pk %q rejected: %v", pk, err)
		}
	}
	for _, pk := range []string{"", "../123", "123&isBusinessApply=Y", " 123", "１２３", "123456789012345678901"} {
		if err := ValidatePublicDataPK(pk); err == nil {
			t.Errorf("unsafe pk %q accepted", pk)
		}
	}
}
