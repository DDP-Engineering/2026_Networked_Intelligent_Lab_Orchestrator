package auth

import (
	"reflect"
	"testing"
)

func TestRolePermissionsMatchTheFixedMatrix(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		role Role
		want []Permission
	}{
		{
			role: RoleViewer,
			want: []Permission{
				PermissionInventoryRead,
				PermissionNetworkRead,
				PermissionStatusRead,
			},
		},
		{
			role: RoleOperator,
			want: []Permission{
				PermissionInventoryRead,
				PermissionNetworkRead,
				PermissionStatusRead,
				PermissionTokensManageSelf,
				PermissionTokensReadSelf,
			},
		},
		{
			role: RoleAdministrator,
			want: []Permission{
				PermissionActivityExport,
				PermissionActivityRead,
				PermissionDiagnosticsManage,
				PermissionInventoryRead,
				PermissionNetworkManage,
				PermissionNetworkRead,
				PermissionStatusRead,
				PermissionTokensManageAll,
				PermissionTokensManageSelf,
				PermissionTokensReadAll,
				PermissionTokensReadSelf,
				PermissionUsersManage,
				PermissionUsersRead,
				PermissionVPNInstall,
				PermissionVPNManage,
			},
		},
	} {
		test := test
		t.Run(string(test.role), func(t *testing.T) {
			t.Parallel()
			if got := test.role.Permissions(); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("%s permissions = %v, want %v", test.role, got, test.want)
			}
			for _, permission := range test.want {
				if !test.role.Has(permission) {
					t.Fatalf("%s does not hold %s", test.role, permission)
				}
			}
		})
	}
}

func TestNoRoleHoldsAPermissionOutsideTheDefinedSet(t *testing.T) {
	t.Parallel()

	defined := make(map[Permission]bool, len(AllPermissions()))
	for _, permission := range AllPermissions() {
		defined[permission] = true
	}
	if len(defined) != 15 {
		t.Fatalf("AllPermissions() has %d entries, want the 15 defined permissions", len(defined))
	}
	for _, role := range []Role{RoleViewer, RoleOperator, RoleAdministrator} {
		for _, permission := range role.Permissions() {
			if !defined[permission] {
				t.Fatalf("%s holds undefined permission %q", role, permission)
			}
		}
	}
	if RoleViewer.Has(PermissionUsersRead) || RoleViewer.Has(PermissionTokensReadSelf) {
		t.Fatal("Viewer holds a user or token permission")
	}
	if RoleOperator.Has(PermissionUsersManage) || RoleOperator.Has(PermissionTokensManageAll) {
		t.Fatal("Operator holds an administrator permission")
	}
}

// The Network service reads are telemetry every role may see; managing an
// interface, a VPN profile or the WireGuard packages changes the appliance's
// connectivity, so those three stay with the Administrator.
func TestNetworkPermissionsAreReadableByEveryRoleAndManagedByAdministratorsOnly(t *testing.T) {
	t.Parallel()

	for _, role := range []Role{RoleViewer, RoleOperator, RoleAdministrator} {
		if !role.Has(PermissionNetworkRead) {
			t.Fatalf("%s cannot read network telemetry", role)
		}
	}
	for _, permission := range []Permission{
		PermissionNetworkManage,
		PermissionVPNManage,
		PermissionVPNInstall,
	} {
		if !RoleAdministrator.Has(permission) {
			t.Fatalf("Administrator does not hold %s", permission)
		}
		if RoleViewer.Has(permission) || RoleOperator.Has(permission) {
			t.Fatalf("%s is held by a role below Administrator", permission)
		}
	}
}

func TestParseRoleAndPermissionRejectUnknownValues(t *testing.T) {
	t.Parallel()

	if _, err := ParseRole("superuser"); err == nil {
		t.Fatal("ParseRole accepted an unknown role")
	}
	if role, err := ParseRole("Administrator"); err != nil || role != RoleAdministrator {
		t.Fatalf("ParseRole(\"Administrator\") = %q, %v", role, err)
	}
	if _, err := ParsePermission("hardware.flash"); err == nil {
		t.Fatal("ParsePermission accepted a permission the model does not define")
	}
	if permission, err := ParsePermission("status.read"); err != nil || permission != PermissionStatusRead {
		t.Fatalf("ParsePermission(\"status.read\") = %q, %v", permission, err)
	}
}

func TestEffectivePermissionsIntersectScopesWithTheOwnerRole(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name   string
		role   Role
		scopes []Permission
		want   []Permission
	}{
		{
			name: "empty scopes are not narrowed",
			role: RoleOperator,
			want: RoleOperator.Permissions(),
		},
		{
			name:   "scopes narrow the role",
			role:   RoleAdministrator,
			scopes: []Permission{PermissionStatusRead},
			want:   []Permission{PermissionStatusRead},
		},
		{
			name:   "scopes cannot exceed the role",
			role:   RoleViewer,
			scopes: []Permission{PermissionUsersManage, PermissionStatusRead},
			want:   []Permission{PermissionStatusRead},
		},
		{
			name:   "a reduced role removes previously granted scopes",
			role:   RoleViewer,
			scopes: []Permission{PermissionTokensManageSelf},
			want:   []Permission{},
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got := EffectivePermissions(test.role, test.scopes)
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("EffectivePermissions(%s, %v) = %v, want %v", test.role, test.scopes, got, test.want)
			}
		})
	}
}

// The activity history and the diagnostics controls reveal account and
// security events and change what the appliance records, so all three of
// their permissions stay with the Administrator (REQ-SEC-d19a7b30).
func TestActivityAndDiagnosticsPermissionsBelongToAdministratorsOnly(t *testing.T) {
	t.Parallel()

	for _, permission := range []Permission{
		PermissionActivityRead,
		PermissionActivityExport,
		PermissionDiagnosticsManage,
	} {
		if !RoleAdministrator.Has(permission) {
			t.Fatalf("Administrator does not hold %s", permission)
		}
		if RoleViewer.Has(permission) || RoleOperator.Has(permission) {
			t.Fatalf("%s is held by a role below Administrator", permission)
		}
	}
}
