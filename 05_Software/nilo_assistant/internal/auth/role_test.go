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
			want: []Permission{PermissionInventoryRead, PermissionStatusRead},
		},
		{
			role: RoleOperator,
			want: []Permission{
				PermissionInventoryRead,
				PermissionStatusRead,
				PermissionTokensManageSelf,
				PermissionTokensReadSelf,
			},
		},
		{
			role: RoleAdministrator,
			want: []Permission{
				PermissionInventoryRead,
				PermissionStatusRead,
				PermissionTokensManageAll,
				PermissionTokensManageSelf,
				PermissionTokensReadAll,
				PermissionTokensReadSelf,
				PermissionUsersManage,
				PermissionUsersRead,
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
	if len(defined) != 8 {
		t.Fatalf("AllPermissions() has %d entries, want the 8 defined permissions", len(defined))
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
