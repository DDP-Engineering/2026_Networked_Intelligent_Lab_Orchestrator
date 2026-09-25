//go:build linux

package nethelper

import (
	"context"
	"fmt"
	"strings"

	"nilo.local/nilo/internal/network"
)

const (
	managementTableName    = "inet nilo_management"
	managementTableComment = "managed by nilo-nethelper for management access"
)

func renderManagementAccess(configuration network.ManagementAccessConfiguration) string {
	var script strings.Builder
	fmt.Fprintf(&script, "table %s {}\n", managementTableName)
	fmt.Fprintf(&script, "delete table %s\n", managementTableName)
	fmt.Fprintf(&script, "table %s {\n\tcomment %q\n", managementTableName, managementTableComment)
	script.WriteString("\tchain input {\n\t\ttype filter hook input priority -5; policy accept;\n")
	// Keep an already accepted connection alive long enough to deliver the
	// pending response. The outer HTTP gate checks every request again, so a
	// keep-alive connection cannot bypass a newly disabled interface.
	fmt.Fprintf(&script, "\t\tct state established,related tcp dport %d accept\n", configuration.Port)
	if len(configuration.Interfaces) > 0 {
		fmt.Fprintf(&script, "\t\tiifname { %s } tcp dport %d accept\n", quotedInterfaceSet(configuration.Interfaces), configuration.Port)
	}
	fmt.Fprintf(&script, "\t\ttcp dport %d drop\n", configuration.Port)
	script.WriteString("\t}\n}\n")
	return script.String()
}

func quotedInterfaceSet(names []string) string {
	quoted := make([]string, 0, len(names))
	for _, name := range names {
		quoted = append(quoted, fmt.Sprintf("%q", name))
	}
	return strings.Join(quoted, ", ")
}

func (n *NetworkManager) ManagementAccessTable(ctx context.Context) (TableState, error) {
	output, err := runNft(ctx, "", "list", "table", "inet", "nilo_management")
	if err != nil {
		if strings.Contains(err.Error(), "No such file or directory") {
			return TableAbsent, nil
		}
		return "", err
	}
	if strings.Contains(output, managementTableComment) {
		return TableOwned, nil
	}
	return TableForeign, nil
}

func (n *NetworkManager) InstallManagementAccess(ctx context.Context, configuration network.ManagementAccessConfiguration) error {
	_, err := runNft(ctx, renderManagementAccess(configuration), "-f", "-")
	return err
}

func (n *NetworkManager) RemoveManagementAccess(ctx context.Context) error {
	state, err := n.ManagementAccessTable(ctx)
	if err != nil || state == TableAbsent {
		return err
	}
	if state != TableOwned {
		return ErrManagementAccessForeign
	}
	_, err = runNft(ctx, "", "delete", "table", "inet", "nilo_management")
	return err
}

var _ ManagementAccessDriver = (*NetworkManager)(nil)
