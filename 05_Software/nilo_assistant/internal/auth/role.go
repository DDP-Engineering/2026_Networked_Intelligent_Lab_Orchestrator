package auth

import (
	"fmt"
	"sort"
	"strings"
)

// Permission names one action an actor may request. The set is fixed and
// compiled in: NILO has no custom-role editor and no policy language, and a
// permission exists only where functionality exists to authorize.
//
// A permission decides whether an actor may *ask* for something. It never
// decides whether an integration operation may run without a human approval:
// that remains the integration app's operation policy, which no role and no
// token scope can bypass.
type Permission string

const (
	// PermissionStatusRead reads the backend status resource.
	PermissionStatusRead Permission = "status.read"
	// PermissionInventoryRead reads the integration and device inventories.
	PermissionInventoryRead Permission = "inventory.read"
	// PermissionTokensReadSelf lists the actor's own API tokens.
	PermissionTokensReadSelf Permission = "tokens.read.self"
	// PermissionTokensManageSelf creates and revokes the actor's own API tokens.
	PermissionTokensManageSelf Permission = "tokens.manage.self"
	// PermissionUsersRead lists identities, their roles and their state.
	PermissionUsersRead Permission = "users.read"
	// PermissionUsersManage creates identities and changes their role, state or password.
	PermissionUsersManage Permission = "users.manage"
	// PermissionTokensReadAll lists every identity's API tokens.
	PermissionTokensReadAll Permission = "tokens.read.all"
	// PermissionTokensManageAll revokes any identity's API token.
	PermissionTokensManageAll Permission = "tokens.manage.all"
	// PermissionNetworkRead reads network status, interface statistics and
	// WireGuard readiness. It is telemetry, so every role carries it.
	PermissionNetworkRead Permission = "network.read"
	// PermissionNetworkManage enables and disables network interfaces.
	PermissionNetworkManage Permission = "network.manage"
	// PermissionVPNManage imports and removes WireGuard profiles, activates and
	// deactivates the tunnel, and chooses the startup profile.
	PermissionVPNManage Permission = "vpn.manage"
	// PermissionVPNInstall installs the WireGuard tooling.
	PermissionVPNInstall Permission = "vpn.install"
	// PermissionActivityRead reads the shared audit and operational-event history.
	PermissionActivityRead Permission = "activity.read"
	// PermissionActivityExport exports a bounded activity selection.
	PermissionActivityExport Permission = "activity.export"
	// PermissionActivityRetentionManage changes the Activity retention policy.
	PermissionActivityRetentionManage Permission = "activity.retention.manage"
	// PermissionSystemPowerManage restarts or powers off the appliance from an
	// interactive browser session. The HTTP boundary does not accept tokens for it.
	PermissionSystemPowerManage Permission = "system.power.manage"
)

// Role is one of the three fixed NILO roles.
type Role string

const (
	RoleViewer        Role = "viewer"
	RoleOperator      Role = "operator"
	RoleAdministrator Role = "administrator"
)

var rolePermissions = map[Role][]Permission{
	RoleViewer: {
		PermissionInventoryRead,
		PermissionNetworkRead,
		PermissionStatusRead,
	},
	RoleOperator: {
		PermissionInventoryRead,
		PermissionNetworkRead,
		PermissionSystemPowerManage,
		PermissionStatusRead,
		PermissionTokensManageSelf,
		PermissionTokensReadSelf,
	},
	RoleAdministrator: {
		PermissionActivityExport,
		PermissionActivityRead,
		PermissionActivityRetentionManage,
		PermissionInventoryRead,
		PermissionNetworkManage,
		PermissionNetworkRead,
		PermissionStatusRead,
		PermissionSystemPowerManage,
		PermissionTokensManageAll,
		PermissionTokensManageSelf,
		PermissionTokensReadAll,
		PermissionTokensReadSelf,
		PermissionUsersManage,
		PermissionUsersRead,
		PermissionVPNInstall,
		PermissionVPNManage,
	},
}

// AllPermissions returns every defined permission in a stable order.
func AllPermissions() []Permission {
	return append([]Permission(nil), rolePermissions[RoleAdministrator]...)
}

// Permissions returns the role's permissions in a stable order. An unknown
// role carries nothing, so a corrupted stored value fails closed.
func (role Role) Permissions() []Permission {
	return append([]Permission(nil), rolePermissions[role]...)
}

// Has reports whether the role carries permission.
func (role Role) Has(permission Permission) bool {
	for _, candidate := range rolePermissions[role] {
		if candidate == permission {
			return true
		}
	}
	return false
}

// ParseRole accepts a case-insensitive role name and rejects anything else.
func ParseRole(value string) (Role, error) {
	role := Role(strings.ToLower(strings.TrimSpace(value)))
	if _, known := rolePermissions[role]; !known {
		return "", fmt.Errorf("unknown role %q", value)
	}
	return role, nil
}

// ParsePermission accepts a defined permission name and rejects anything else,
// so a scope list can never name a capability the model does not implement.
func ParsePermission(value string) (Permission, error) {
	candidate := Permission(strings.ToLower(strings.TrimSpace(value)))
	for _, permission := range rolePermissions[RoleAdministrator] {
		if permission == candidate {
			return permission, nil
		}
	}
	return "", fmt.Errorf("unknown permission %q", value)
}

// EffectivePermissions intersects a role's permissions with a scope list.
// An empty scope list means "not narrowed" and yields the whole role, which is
// what keeps tokens created before scopes existed working unchanged. Because
// the result is an intersection, a scope can only ever remove a permission.
func EffectivePermissions(role Role, scopes []Permission) []Permission {
	granted := role.Permissions()
	if len(scopes) == 0 {
		return granted
	}
	requested := make(map[Permission]bool, len(scopes))
	for _, scope := range scopes {
		requested[scope] = true
	}
	effective := make([]Permission, 0, len(granted))
	for _, permission := range granted {
		if requested[permission] {
			effective = append(effective, permission)
		}
	}
	sort.Slice(effective, func(first, second int) bool { return effective[first] < effective[second] })
	return effective
}

// FormatPermissions renders a scope list for storage as a comma-separated
// string; an empty list stores as the empty string.
func FormatPermissions(permissions []Permission) string {
	names := make([]string, 0, len(permissions))
	for _, permission := range permissions {
		names = append(names, string(permission))
	}
	sort.Strings(names)
	return strings.Join(names, ",")
}

// ParsePermissionList reads a stored or command-line comma-separated scope
// list. Unknown names are rejected rather than silently dropped.
func ParsePermissionList(value string) ([]Permission, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, nil
	}
	seen := make(map[Permission]bool)
	permissions := make([]Permission, 0, 4)
	for _, field := range strings.Split(trimmed, ",") {
		if strings.TrimSpace(field) == "" {
			continue
		}
		permission, err := ParsePermission(field)
		if err != nil {
			return nil, err
		}
		if seen[permission] {
			continue
		}
		seen[permission] = true
		permissions = append(permissions, permission)
	}
	sort.Slice(permissions, func(first, second int) bool { return permissions[first] < permissions[second] })
	return permissions, nil
}
