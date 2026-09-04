package nethelper

import (
	"fmt"
	"net/netip"
	"strings"

	"nilo.local/nilo/internal/network"
)

// Rendering the inventory into nftables. The whole owned table is written in
// one script that nft applies as one transaction: the old table is removed
// and the new one created together, so there is no moment with half a table.
// The renderer has no host knowledge, which is what lets it be tested here.

const (
	ownedTableName    = "inet nilo"
	ownedTableComment = "managed by nilo-nethelper"
)

// renderNftables writes the script for one inventory.
func renderNftables(inventory ForwardingInventory) string {
	var script strings.Builder
	// Creating the table when absent makes the deletion that follows valid;
	// both are part of the same transaction as the definition.
	fmt.Fprintf(&script, "table %s {}\n", ownedTableName)
	fmt.Fprintf(&script, "delete table %s\n", ownedTableName)
	if len(inventory.Plans) == 0 {
		return script.String()
	}
	// Where NILO turned forwarding on, nothing it does not grant is forwarded:
	// before NILO nothing was forwarded either. Where the operator already
	// forwarded, NILO grants and guards its own scopes and closes nothing else.
	policy := "accept"
	if inventory.Switch.Owned {
		policy = "drop"
	}
	fmt.Fprintf(&script, "table %s {\n\tcomment %q\n", ownedTableName, ownedTableComment)
	fmt.Fprintf(&script, "\tchain forward {\n\t\ttype filter hook forward priority filter; policy %s;\n", policy)
	// Replies of a flow a grant admitted are let back through; the packets
	// the client sends still pass the grants every time, so taking a grant
	// away ends the session at the very next request.
	script.WriteString("\t\tct state established,related ct direction reply accept\n")
	for _, plan := range inventory.Plans {
		fmt.Fprintf(&script, "\t\tjump %s\n", chainName(plan.DeviceNetworkID))
	}
	script.WriteString("\t}\n")
	script.WriteString("\tchain prerouting {\n\t\ttype nat hook prerouting priority dstnat; policy accept;\n")
	for _, plan := range inventory.Plans {
		for _, resource := range plan.Resources {
			if resource.Kind == network.ResourceDestinationNAT {
				script.WriteString("\t\t" + renderDNAT(resource) + "\n")
			}
		}
	}
	script.WriteString("\t}\n")
	script.WriteString("\tchain postrouting {\n\t\ttype nat hook postrouting priority srcnat; policy accept;\n")
	for _, plan := range inventory.Plans {
		for _, resource := range plan.Resources {
			if resource.Kind == network.ResourceSourceNAT {
				fmt.Fprintf(&script, "\t\toifname %q ip daddr %s snat ip to %s\n", resource.Egress, prefixText(resource.Destination), prefixText(resource.Translated))
			}
		}
	}
	script.WriteString("\t}\n")
	for _, plan := range inventory.Plans {
		renderPlanChain(&script, plan)
	}
	script.WriteString("}\n")
	return script.String()
}

// renderPlanChain writes one device network's chain: grants first, then the
// guards, so an authorized packet is accepted before anything can drop it and
// everything else toward the scope is dropped.
func renderPlanChain(script *strings.Builder, plan network.ForwardingPlan) {
	fmt.Fprintf(script, "\tchain %s {\n", chainName(plan.DeviceNetworkID))
	var translated *network.Resource
	egress := ""
	for index := range plan.Resources {
		resource := &plan.Resources[index]
		if resource.Kind == network.ResourceDestinationNAT {
			translated = resource
		}
		if egress == "" && resource.Egress != "" {
			egress = resource.Egress
		}
	}
	for _, resource := range plan.Resources {
		if resource.Kind != network.ResourceForwardGrant {
			continue
		}
		destination := "ip daddr " + prefixText(resource.Destination)
		if translated != nil {
			// A translated grant is matched on the address the client actually
			// used, so the internal address is never a way in.
			destination = "ct original ip daddr " + prefixText(resource.Destination)
		}
		fmt.Fprintf(script, "\t\tiifname %q oifname %q ip saddr %s %s %s accept\n",
			resource.Ingress, resource.Egress, prefixText(resource.Source), destination, renderProtocol(resource))
	}
	if translated != nil {
		fmt.Fprintf(script, "\t\toifname %q ip daddr %s drop\n", translated.Egress, prefixText(translated.Translated))
	}
	for _, resource := range plan.Resources {
		switch resource.Kind {
		case network.ResourceDenyGuard:
			fmt.Fprintf(script, "\t\tip daddr %s drop\n", prefixText(resource.Destination))
		case network.ResourceIPv6Deny:
			if egress != "" {
				fmt.Fprintf(script, "\t\tmeta nfproto ipv6 oifname %q drop\n", egress)
			}
		}
	}
	script.WriteString("\t}\n")
}

func renderDNAT(resource network.Resource) string {
	if resource.Destination.Bits() == 32 && resource.Translated.Bits() == 32 {
		return fmt.Sprintf("ip daddr %s dnat ip to %s", prefixText(resource.Destination), prefixText(resource.Translated))
	}
	return fmt.Sprintf("ip daddr %s dnat ip prefix to ip daddr map { %s : %s }",
		prefixText(resource.Destination), resource.Destination.Masked().String(), resource.Translated.Masked().String())
}

func renderProtocol(resource network.Resource) string {
	switch resource.Protocol {
	case "tcp", "udp":
		if len(resource.Ports) == 0 {
			return "meta l4proto " + resource.Protocol
		}
		ports := make([]string, 0, len(resource.Ports))
		for _, port := range resource.Ports {
			ports = append(ports, fmt.Sprint(port))
		}
		return fmt.Sprintf("%s dport { %s }", resource.Protocol, strings.Join(ports, ", "))
	default:
		return "meta l4proto " + resource.Protocol
	}
}

// prefixText writes a host prefix as its address and anything else as a
// prefix, which is how nft expects them.
func prefixText(prefix netip.Prefix) string {
	if prefix.Bits() == 32 || prefix.Bits() == 128 {
		return prefix.Addr().String()
	}
	return prefix.Masked().String()
}

// chainName is a valid nft identifier carrying the device network's identity.
func chainName(deviceNetworkID string) string {
	var name strings.Builder
	name.WriteString("dn_")
	for _, r := range deviceNetworkID {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_':
			name.WriteRune(r)
		default:
			name.WriteByte('_')
		}
	}
	text := name.String()
	if len(text) > 60 {
		text = text[:60]
	}
	return text
}
