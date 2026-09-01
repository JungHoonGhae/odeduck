//go:build windows

package providerauth

import (
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestCredentialFileDACLAllowsOnlyCurrentUserAndSystem(t *testing.T) {
	isolateConfigHome(t)
	if err := Set("safetykorea", Credential{Key: "SAFETY-SECRET-123"}); err != nil {
		t.Fatal(err)
	}
	path, err := credentialPath("safetykorea")
	if err != nil {
		t.Fatal(err)
	}
	descriptor, err := windows.GetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.OWNER_SECURITY_INFORMATION,
	)
	if err != nil {
		t.Fatal(err)
	}
	control, _, err := descriptor.Control()
	if err != nil {
		t.Fatal(err)
	}
	if control&windows.SE_DACL_PROTECTED == 0 {
		t.Fatal("credential DACL still inherits broader parent permissions")
	}
	dacl, _, err := descriptor.DACL()
	if err != nil || dacl == nil {
		t.Fatalf("credential DACL missing: error=%v", err)
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	system, err := windows.StringToSid("S-1-5-18")
	if err != nil {
		t.Fatal(err)
	}
	if dacl.AceCount != 2 {
		t.Fatalf("credential DACL ACE count=%d, want current user and SYSTEM", dacl.AceCount)
	}
	for i := uint16(0); i < dacl.AceCount; i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, uint32(i), &ace); err != nil {
			t.Fatal(err)
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || ace.Mask == 0 {
			t.Fatalf("unexpected ACE[%d]: type=%d mask=%x", i, ace.Header.AceType, ace.Mask)
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if !sid.Equals(user.User.Sid) && !sid.Equals(system) {
			t.Fatalf("credential DACL grants unexpected SID %s", sid.String())
		}
	}
}
