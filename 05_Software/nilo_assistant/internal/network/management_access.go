package network

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ManagementAccessState is the durable state of the management listener
// allowlist. A pending configuration must be confirmed through one of its
// allowed interfaces before its deadline or the last known-good configuration
// is restored.
type ManagementAccessState string

const (
	ManagementAccessApplied   ManagementAccessState = "applied"
	ManagementAccessPending   ManagementAccessState = "pending"
	ManagementAccessFailed    ManagementAccessState = "failed"
	ManagementAccessRecovered ManagementAccessState = "recovered"
)

// ManagementAccessConfirmationWindow is deliberately long enough for an
// administrator to move the browser to another already-configured path.
const ManagementAccessConfirmationWindow = 90 * time.Second

var (
	ErrManagementAccessRequired   = errors.New("at least one management interface is required")
	ErrManagementAccessUnsafe     = errors.New("management access configuration has no usable interface")
	ErrCurrentAccessConfirmation  = errors.New("disabling the current management interface requires explicit confirmation")
	ErrManagementAccessPending    = errors.New("a management access change is already pending")
	ErrManagementAccessNotPending = errors.New("no management access change is pending")
	ErrManagementReconfigure      = errors.New("persistent management access differs; pass --management-reconfigure to stage a recoverable console change")
	managementInterfaceName       = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,14}$`)
)

// ManagementInterfaceBinding keeps the physical identity and last observed
// Linux name of one management interface. AdapterID is empty for virtual or
// not-yet-resolved legacy interfaces.
type ManagementInterfaceBinding struct {
	AdapterID     string `json:"adapter_id"`
	LastKnownName string `json:"last_known_name"`
}

// ManagementAccessConfiguration is the stable listener identity. Interfaces
// is the current helper-facing name list; Bindings is its durable identity.
type ManagementAccessConfiguration struct {
	Interfaces []string
	Bindings   []ManagementInterfaceBinding `json:"-"`
	Port       int
	TLSNames   []string
}

// Validate rejects configurations that could become a wildcard listener
// without an explicit, meaningful allowlist.
func (configuration ManagementAccessConfiguration) Validate() error {
	return configuration.validate(true)
}

// ValidateFirewall permits an empty effective allowlist to close admission
// when saved adapters are absent. User configuration still requires a selection.
func (configuration ManagementAccessConfiguration) ValidateFirewall() error {
	return configuration.validate(false)
}

func (configuration ManagementAccessConfiguration) validate(requireInterface bool) error {
	configuration = canonicalManagementConfiguration(configuration)
	if configuration.Port < 1 || configuration.Port > 65535 {
		return errors.New("management port must be between 1 and 65535")
	}
	if requireInterface && len(configuration.Interfaces) == 0 {
		return ErrManagementAccessRequired
	}
	seen := map[string]bool{}
	for _, binding := range configuration.Bindings {
		name := binding.LastKnownName
		if !managementInterfaceName.MatchString(name) || name == "lo" {
			return fmt.Errorf("invalid management interface %q", name)
		}
		key := "name:" + name
		if requireInterface && binding.AdapterID != "" {
			key = "adapter:" + binding.AdapterID
		}
		if seen[key] {
			return fmt.Errorf("duplicate management interface %q", name)
		}
		seen[key] = true
	}
	if len(configuration.TLSNames) == 0 {
		return errors.New("at least one management TLS name is required")
	}
	tlsSeen := map[string]bool{}
	for _, name := range configuration.TLSNames {
		if !validManagementTLSName(name) {
			return fmt.Errorf("invalid management TLS name %q", name)
		}
		key := strings.ToLower(name)
		if tlsSeen[key] {
			return fmt.Errorf("duplicate management TLS name %q", name)
		}
		tlsSeen[key] = true
	}
	return nil
}

func validManagementTLSName(name string) bool {
	if name == "" || name != strings.TrimSpace(name) || strings.ContainsAny(name, "\x00/\\") {
		return false
	}
	if _, err := netip.ParseAddr(name); err == nil {
		return true
	}
	if len(name) > 253 {
		return false
	}
	for _, label := range strings.Split(name, ".") {
		if len(label) == 0 || len(label) > 63 || !asciiAlphaNumeric(label[0]) || !asciiAlphaNumeric(label[len(label)-1]) {
			return false
		}
		for index := 1; index < len(label)-1; index++ {
			if !asciiAlphaNumeric(label[index]) && label[index] != '-' {
				return false
			}
		}
	}
	return true
}

func asciiAlphaNumeric(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' || value >= '0' && value <= '9'
}

func canonicalManagementConfiguration(configuration ManagementAccessConfiguration) ManagementAccessConfiguration {
	bindings := append([]ManagementInterfaceBinding(nil), configuration.Bindings...)
	if len(bindings) == 0 {
		for _, name := range configuration.Interfaces {
			bindings = append(bindings, ManagementInterfaceBinding{LastKnownName: name})
		}
	}
	seen := map[string]bool{}
	canonical := make([]ManagementInterfaceBinding, 0, len(bindings))
	for _, binding := range bindings {
		binding.AdapterID = canonicalAdapterID(binding.AdapterID)
		binding.LastKnownName = strings.TrimSpace(binding.LastKnownName)
		key := "name:" + binding.LastKnownName
		if binding.AdapterID != "" {
			key = "adapter:" + binding.AdapterID
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		canonical = append(canonical, binding)
	}
	sort.Slice(canonical, func(first, second int) bool {
		firstKey := canonical[first].AdapterID + "\x00" + canonical[first].LastKnownName
		secondKey := canonical[second].AdapterID + "\x00" + canonical[second].LastKnownName
		return firstKey < secondKey
	})
	configuration.Bindings = canonical
	configuration.Interfaces = make([]string, 0, len(canonical))
	for _, binding := range canonical {
		configuration.Interfaces = append(configuration.Interfaces, binding.LastKnownName)
	}
	configuration.TLSNames = append([]string(nil), configuration.TLSNames...)
	sort.Strings(configuration.TLSNames)
	return configuration
}

func sameManagementConfiguration(first, second ManagementAccessConfiguration) bool {
	first = canonicalManagementConfiguration(first)
	second = canonicalManagementConfiguration(second)
	firstClaims := managementBindingKeys(first.Bindings)
	secondClaims := managementBindingKeys(second.Bindings)
	return first.Port == second.Port && strings.Join(firstClaims, "\x00") == strings.Join(secondClaims, "\x00") &&
		strings.Join(first.TLSNames, "\x00") == strings.Join(second.TLSNames, "\x00")
}

func managementBindingKeys(bindings []ManagementInterfaceBinding) []string {
	keys := make([]string, 0, len(bindings))
	for _, binding := range bindings {
		key := "name:" + binding.LastKnownName
		if binding.AdapterID != "" {
			key = "adapter:" + canonicalAdapterID(binding.AdapterID)
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func resolveManagementConfiguration(configuration ManagementAccessConfiguration, interfaces []Interface) ManagementAccessConfiguration {
	configuration = canonicalManagementConfiguration(configuration)
	for index, binding := range configuration.Bindings {
		for _, candidate := range interfaces {
			matches := binding.AdapterID != "" && canonicalAdapterID(candidate.AdapterID) == binding.AdapterID
			if binding.AdapterID == "" && candidate.Name == binding.LastKnownName {
				matches = true
			}
			if !matches {
				continue
			}
			configuration.Bindings[index].LastKnownName = candidate.Name
			if (candidate.Kind == KindEthernet || candidate.Kind == KindWiFi) && candidate.AdapterID != "" {
				configuration.Bindings[index].AdapterID = canonicalAdapterID(candidate.AdapterID)
			}
			break
		}
	}
	return canonicalManagementConfiguration(configuration)
}

// effectiveManagementConfiguration projects durable identity onto the current
// inventory. Last-known names never authorize replacement physical adapters.
func effectiveManagementConfiguration(configuration ManagementAccessConfiguration, interfaces []Interface) ManagementAccessConfiguration {
	configuration = resolveManagementConfiguration(configuration, interfaces)
	names := make([]string, 0, len(configuration.Bindings))
	for _, binding := range configuration.Bindings {
		for _, candidate := range interfaces {
			if candidate.Name == binding.LastKnownName && (binding.AdapterID == "" || binding.AdapterID == canonicalAdapterID(candidate.AdapterID)) {
				names = append(names, candidate.Name)
				break
			}
		}
	}
	configuration.Interfaces = names
	configuration.Bindings = nil
	return configuration
}

// ManagementAccessRecord is the one durable management-listener decision.
// LastKnownGood is never empty after initial configuration.
type ManagementAccessRecord struct {
	Configuration ManagementAccessConfiguration
	LastKnownGood ManagementAccessConfiguration
	State         ManagementAccessState
	ConfirmBy     time.Time
	Detail        string
	Outcome       ManagementAccessOutcome
	UpdatedAt     time.Time
}

// ManagementAccessOutcomeCode is a stable, localizable reason for a persisted
// transition. Detail remains an English console diagnostic and is never
// rendered directly by the browser.
type ManagementAccessOutcomeCode string

const (
	ManagementOutcomeStartupRecovered      ManagementAccessOutcomeCode = "startup_recovered"
	ManagementOutcomeConsolePending        ManagementAccessOutcomeCode = "console_confirmation_required"
	ManagementOutcomeRoleConflictRecovered ManagementAccessOutcomeCode = "role_conflict_recovered"
	ManagementOutcomeProposedRestored      ManagementAccessOutcomeCode = "proposed_change_restored"
	ManagementOutcomeRecoveryFailed        ManagementAccessOutcomeCode = "recovery_failed"
	ManagementOutcomeAutomaticFailed       ManagementAccessOutcomeCode = "automatic_recovery_failed"
	ManagementOutcomeAutomaticRestored     ManagementAccessOutcomeCode = "automatic_change_restored"
)

// Valid reports whether the code is one of the stable browser-facing outcomes.
func (code ManagementAccessOutcomeCode) Valid() bool {
	switch code {
	case ManagementOutcomeStartupRecovered,
		ManagementOutcomeConsolePending,
		ManagementOutcomeRoleConflictRecovered,
		ManagementOutcomeProposedRestored,
		ManagementOutcomeRecoveryFailed,
		ManagementOutcomeAutomaticFailed,
		ManagementOutcomeAutomaticRestored:
		return true
	default:
		return false
	}
}

// ManagementAccessOutcome carries only safe adapter and resource labels.
type ManagementAccessOutcome struct {
	Code         ManagementAccessOutcomeCode `json:"code"`
	Interface    string                      `json:"interface"`
	ResourceName string                      `json:"resource_name"`
}

// ManagementAccessStore persists the allowlist in the existing NILO database.
type ManagementAccessStore interface {
	LoadManagementAccess(context.Context) (ManagementAccessRecord, bool, error)
	SaveManagementAccess(context.Context, ManagementAccessRecord) error
}

// ManagementAccessFirewall is implemented by the privileged network helper.
// The unprivileged core can request only a validated port and interface list,
// never raw rules or commands.
type ManagementAccessFirewall interface {
	ApplyManagementAccess(context.Context, ManagementAccessConfiguration) error
}

// ManagementAccessInterface is the public state shown for one detected device.
type ManagementAccessInterface struct {
	Name                    string      `json:"name"`
	AdapterID               string      `json:"adapter_id"`
	Kind                    Kind        `json:"kind"`
	State                   State       `json:"state"`
	IPv4                    []string    `json:"ipv4"`
	IPv6                    []string    `json:"ipv6"`
	Enabled                 bool        `json:"enabled"`
	Current                 bool        `json:"current"`
	Usable                  bool        `json:"usable"`
	Role                    AdapterRole `json:"role"`
	RoleAvailable           bool        `json:"role_available"`
	RoleDetail              string      `json:"role_detail"`
	ConflictingResourceID   string      `json:"conflicting_resource_id"`
	ConflictingResourceName string      `json:"conflicting_resource_name"`
}

// ManagementURL is a stable URL backed by a certificate identity.
type ManagementURL struct {
	URL  string `json:"url"`
	Kind string `json:"kind"`
}

// ManagementAccessStatus is safe for API and logs: it contains no Wi-Fi
// passwords, WireGuard keys, or other secret material.
type ManagementAccessStatus struct {
	State            ManagementAccessState
	Port             int
	Interfaces       []ManagementAccessInterface
	Configured       []string
	URLs             []ManagementURL
	TLSNames         []string
	TLSMissing       []string
	ConfirmBy        time.Time
	Detail           string
	Outcome          ManagementAccessOutcome
	CurrentInterface string
}

// ManagementAccessOptions composes the listener policy with observation,
// persistence, privileged firewall application, and its clock.
type ManagementAccessOptions struct {
	Provider      Provider
	Store         ManagementAccessStore
	Firewall      ManagementAccessFirewall
	Now           func() time.Time
	Initial       *ManagementAccessConfiguration
	Reconfigure   bool
	TLSMissing    []string
	ConfirmWindow time.Duration
	Coordinator   *AdapterRoleCoordinator
}

// ManagementAccessService owns the active allowlist used by both the socket
// gate and the administrator API.
type ManagementAccessService struct {
	mu            sync.RWMutex
	changeMu      sync.Mutex
	provider      Provider
	store         ManagementAccessStore
	firewall      ManagementAccessFirewall
	now           func() time.Time
	confirmWindow time.Duration
	record        ManagementAccessRecord
	interfaces    []Interface
	allowed       map[netip.Addr]string
	tlsMissing    []string
	coordinator   *AdapterRoleCoordinator
}

// NewManagementAccessService loads or deliberately initializes the durable
// configuration, restores an interrupted pending change, applies the helper
// firewall, and observes the current interfaces before the socket can listen.
func NewManagementAccessService(ctx context.Context, options ManagementAccessOptions) (*ManagementAccessService, error) {
	if options.Provider == nil || options.Store == nil || options.Firewall == nil {
		return nil, errors.New("management access requires a provider, store and firewall")
	}
	if options.Now == nil {
		options.Now = func() time.Time { return time.Now().UTC() }
	}
	if options.ConfirmWindow <= 0 {
		options.ConfirmWindow = ManagementAccessConfirmationWindow
	}
	if options.Coordinator == nil {
		options.Coordinator = NewAdapterRoleCoordinator()
	}
	needsSave := false
	record, present, err := options.Store.LoadManagementAccess(ctx)
	if err != nil {
		return nil, fmt.Errorf("load management access: %w", err)
	}
	if record.State == ManagementAccessPending || record.State == ManagementAccessFailed {
		if err := record.LastKnownGood.Validate(); err != nil {
			return nil, fmt.Errorf("pending management access has no valid recovery configuration: %w", err)
		}
		record.Configuration = record.LastKnownGood
		record.State = ManagementAccessRecovered
		record.ConfirmBy = time.Time{}
		record.Detail = "an unconfirmed management access change was restored during startup"
		record.Outcome = ManagementAccessOutcome{Code: ManagementOutcomeStartupRecovered}
		record.UpdatedAt = options.Now().UTC()
		needsSave = true
	}
	observed, err := options.Provider.Interfaces(ctx)
	if err != nil {
		return nil, fmt.Errorf("observe management interfaces: %w", err)
	}
	if options.Initial != nil {
		configuration := resolveManagementConfiguration(*options.Initial, observed)
		if err := configuration.Validate(); err != nil {
			return nil, err
		}
		if !managementConfigurationHasUsableInterface(observed, effectiveManagementConfiguration(configuration, observed).Interfaces) {
			return nil, ErrManagementAccessUnsafe
		}
		switch {
		case !present:
			record = ManagementAccessRecord{
				Configuration: configuration, LastKnownGood: configuration,
				State: ManagementAccessApplied, UpdatedAt: options.Now().UTC(),
			}
		case sameManagementConfiguration(configuration, record.Configuration):
			// Repeated service flags matching persistent state are harmless and
			// must not erase a recovered status or its last-known-good record.
		case !options.Reconfigure:
			return nil, ErrManagementReconfigure
		default:
			record = ManagementAccessRecord{
				Configuration: configuration, LastKnownGood: record.Configuration,
				State: ManagementAccessPending, ConfirmBy: options.Now().UTC().Add(options.ConfirmWindow),
				UpdatedAt: options.Now().UTC(), Detail: "console management reconfiguration requires confirmation",
				Outcome: ManagementAccessOutcome{Code: ManagementOutcomeConsolePending},
			}
		}
		needsSave = true
		present = true
	}
	if !present {
		return nil, errors.New("no persistent management access configuration; specify --management-interface at the appliance console")
	}
	resolvedConfiguration := resolveManagementConfiguration(record.Configuration, observed)
	resolvedLastKnownGood := resolveManagementConfiguration(record.LastKnownGood, observed)
	bindingsChanged := !sameManagementBindings(record.Configuration, resolvedConfiguration) ||
		!sameManagementBindings(record.LastKnownGood, resolvedLastKnownGood)
	record.Configuration = resolvedConfiguration
	record.LastKnownGood = resolvedLastKnownGood
	record, recoveredConflict, err := reconcileManagementRoleConflicts(record, observed, options.Coordinator, options.Now().UTC())
	if err != nil {
		return nil, err
	}
	if recoveredConflict {
		needsSave = true
	}
	if err := record.Configuration.Validate(); err != nil {
		return nil, fmt.Errorf("invalid saved management access: %w", err)
	}
	if bindingsChanged {
		record.UpdatedAt = options.Now().UTC()
		needsSave = true
	}
	startupReservation := "management-access-startup-" + randomIdentity()
	if err := options.Coordinator.Reserve(startupReservation, managementRecordRoleClaims(record)); err != nil {
		return nil, err
	}
	defer options.Coordinator.Release(startupReservation)
	// Persist the authoritative decision before changing the privileged
	// firewall. A failed initial save therefore leaves no transient exposure.
	if needsSave {
		if err := options.Store.SaveManagementAccess(ctx, record); err != nil {
			return nil, fmt.Errorf("save resolved management access: %w", err)
		}
	}
	if err := options.Firewall.ApplyManagementAccess(ctx, effectiveManagementConfiguration(record.Configuration, observed)); err != nil {
		return nil, fmt.Errorf("apply management firewall: %w", err)
	}
	service := &ManagementAccessService{
		provider: options.Provider, store: options.Store, firewall: options.Firewall,
		now: options.Now, confirmWindow: options.ConfirmWindow, record: record,
		allowed: map[netip.Addr]string{}, tlsMissing: append([]string(nil), options.TLSMissing...),
		interfaces:  cloneInterfaces(observed),
		coordinator: options.Coordinator,
	}
	service.rebuildAllowedLocked()
	service.coordinator.RegisterClaimSource(service.adapterRoleClaims)
	return service, nil
}

func reconcileManagementRoleConflicts(
	record ManagementAccessRecord,
	interfaces []Interface,
	coordinator *AdapterRoleCoordinator,
	now time.Time,
) (ManagementAccessRecord, bool, error) {
	conflicts := coordinator.Conflicts(managementRoleClaims(record.Configuration))
	if len(conflicts) == 0 {
		return record, false, nil
	}
	conflicting := map[string]*AdapterRoleConflictError{}
	for _, conflict := range conflicts {
		conflicting[canonicalAdapterID(conflict.AdapterID)] = conflict
	}
	clean := withoutManagementRoleConflicts(record.Configuration, conflicting)
	if !managementConfigurationHasUsableInterface(interfaces, effectiveManagementConfiguration(clean, interfaces).Interfaces) {
		return record, false, conflicts[0]
	}
	good := withoutManagementRoleConflicts(record.LastKnownGood, conflicting)
	if len(good.Interfaces) == 0 {
		good = clean
	}
	first := conflicts[0]
	detail := fmt.Sprintf("management adapter %s conflicted with the device role and was removed", first.Interface)
	if first.ConflictingResourceName != "" {
		detail = fmt.Sprintf("management adapter %s conflicted with device network %q and was removed", first.Interface, first.ConflictingResourceName)
	}
	record.Configuration = clean
	record.LastKnownGood = good
	record.State = ManagementAccessRecovered
	record.ConfirmBy = time.Time{}
	record.Detail = detail
	record.Outcome = ManagementAccessOutcome{
		Code: ManagementOutcomeRoleConflictRecovered, Interface: first.Interface,
		ResourceName: first.ConflictingResourceName,
	}
	record.UpdatedAt = now
	return record, true, nil
}

func withoutManagementRoleConflicts(
	configuration ManagementAccessConfiguration,
	conflicts map[string]*AdapterRoleConflictError,
) ManagementAccessConfiguration {
	configuration = canonicalManagementConfiguration(configuration)
	bindings := make([]ManagementInterfaceBinding, 0, len(configuration.Bindings))
	for _, binding := range configuration.Bindings {
		if _, conflict := conflicts[canonicalAdapterID(binding.AdapterID)]; binding.AdapterID != "" && conflict {
			continue
		}
		bindings = append(bindings, binding)
	}
	configuration.Bindings = bindings
	configuration.Interfaces = nil
	return canonicalManagementConfiguration(configuration)
}

func managementRoleClaims(configuration ManagementAccessConfiguration) []AdapterRoleClaim {
	configuration = canonicalManagementConfiguration(configuration)
	claims := make([]AdapterRoleClaim, 0, len(configuration.Bindings))
	for _, binding := range configuration.Bindings {
		if binding.AdapterID == "" {
			continue
		}
		claims = append(claims, AdapterRoleClaim{
			AdapterID: binding.AdapterID, Interface: binding.LastKnownName, Role: AdapterRoleManagement,
		})
	}
	return claims
}

func managementRecordRoleClaims(record ManagementAccessRecord) []AdapterRoleClaim {
	claims := managementRoleClaims(record.Configuration)
	if record.State == ManagementAccessPending || record.State == ManagementAccessFailed {
		claims = append(claims, managementRoleClaims(record.LastKnownGood)...)
	}
	return claims
}

func (service *ManagementAccessService) adapterRoleClaims() []AdapterRoleClaim {
	service.mu.RLock()
	defer service.mu.RUnlock()
	return managementRecordRoleClaims(service.record)
}

func sameManagementBindings(first, second ManagementAccessConfiguration) bool {
	first = canonicalManagementConfiguration(first)
	second = canonicalManagementConfiguration(second)
	if len(first.Bindings) != len(second.Bindings) {
		return false
	}
	for index := range first.Bindings {
		if first.Bindings[index] != second.Bindings[index] {
			return false
		}
	}
	return true
}

// Refresh updates the address cache used by the pre-HTTP socket gate. It does
// not alter the allowlist, so DHCP renewal and interface return need no restart
// or configuration write.
func (service *ManagementAccessService) Refresh(ctx context.Context) error {
	service.changeMu.Lock()
	defer service.changeMu.Unlock()
	return service.refresh(ctx)
}

func (service *ManagementAccessService) refresh(ctx context.Context) error {
	interfaces, err := service.provider.Interfaces(ctx)
	if err != nil {
		return fmt.Errorf("refresh management interfaces: %w", err)
	}
	service.mu.Lock()
	previous := service.record
	previousEffective := effectiveManagementConfiguration(previous.Configuration, service.interfaces)
	// Revoke stale destinations even if subsequent persistence or firewall
	// reconciliation fails. New admission follows successful reconciliation.
	observedAllowed := managementAllowedAddresses(previous.Configuration, interfaces)
	for address := range service.allowed {
		if _, present := observedAllowed[address]; !present {
			delete(service.allowed, address)
		}
	}
	service.mu.Unlock()
	candidate := previous
	candidate.Configuration = resolveManagementConfiguration(candidate.Configuration, interfaces)
	candidate.LastKnownGood = resolveManagementConfiguration(candidate.LastKnownGood, interfaces)
	changed := !sameManagementBindings(previous.Configuration, candidate.Configuration) ||
		!sameManagementBindings(previous.LastKnownGood, candidate.LastKnownGood)
	candidate, recovered, err := reconcileManagementRoleConflicts(candidate, interfaces, service.coordinator, service.now().UTC())
	if err != nil {
		return err
	}
	changed = changed || recovered
	changed = changed || !sameManagementConfiguration(previousEffective, effectiveManagementConfiguration(candidate.Configuration, interfaces))
	if changed {
		candidate.UpdatedAt = service.now().UTC()
		if err := service.coordinator.CheckAndRun(managementRecordRoleClaims(candidate), func() error {
			if err := service.firewall.ApplyManagementAccess(ctx, effectiveManagementConfiguration(candidate.Configuration, interfaces)); err != nil {
				return err
			}
			if err := service.store.SaveManagementAccess(ctx, candidate); err != nil {
				return service.publishFailedUpdate(
					context.WithoutCancel(ctx), previous.Configuration, candidate.Configuration, interfaces, err,
				)
			}
			service.mu.Lock()
			service.interfaces = cloneInterfaces(interfaces)
			service.record = candidate
			service.rebuildAllowedLocked()
			service.mu.Unlock()
			return nil
		}); err != nil {
			return fmt.Errorf("reconcile management interfaces: %w", err)
		}
		return nil
	}
	service.mu.Lock()
	service.interfaces = cloneInterfaces(interfaces)
	service.record = candidate
	service.rebuildAllowedLocked()
	service.mu.Unlock()
	return nil
}

func cloneInterfaces(interfaces []Interface) []Interface {
	result := append([]Interface(nil), interfaces...)
	for index := range result {
		result[index].IPv4 = append([]string(nil), result[index].IPv4...)
		result[index].IPv6 = append([]string(nil), result[index].IPv6...)
	}
	return result
}

func (service *ManagementAccessService) rebuildAllowedLocked() {
	service.allowed = managementAllowedAddresses(service.record.Configuration, service.interfaces)
}

func managementAllowedAddresses(configuration ManagementAccessConfiguration, interfaces []Interface) map[netip.Addr]string {
	configured := map[string]bool{}
	for _, name := range effectiveManagementConfiguration(configuration, interfaces).Interfaces {
		configured[name] = true
	}
	allowed := map[netip.Addr]string{}
	for _, candidate := range interfaces {
		if !configured[candidate.Name] || candidate.State != StateConnected {
			continue
		}
		for _, text := range append(append([]string(nil), candidate.IPv4...), candidate.IPv6...) {
			prefix, err := netip.ParsePrefix(text)
			if err == nil {
				allowed[prefix.Addr().Unmap()] = candidate.Name
			}
		}
	}
	return allowed
}

// Allows reports whether a connection's local destination address currently
// belongs to an enabled interface. It performs no I/O and is safe in Accept.
func (service *ManagementAccessService) Allows(address netip.Addr) bool {
	service.mu.RLock()
	defer service.mu.RUnlock()
	_, ok := service.allowed[address.Unmap()]
	return ok
}

// ConfiguredInterfaces returns a copy for the existing network-change safety
// guard, so disabling an adapter cannot ignore the listener allowlist.
func (service *ManagementAccessService) ConfiguredInterfaces() []string {
	service.mu.RLock()
	defer service.mu.RUnlock()
	return effectiveManagementConfiguration(service.record.Configuration, service.interfaces).Interfaces
}

func (service *ManagementAccessService) currentInterface(address netip.Addr) string {
	if !address.IsValid() {
		return ""
	}
	return service.allowed[address.Unmap()]
}

// Status returns the detected inventory and stable URLs. DHCP addresses remain
// observations on interfaces and are never promoted into permanent URLs.
func (service *ManagementAccessService) Status(ctx context.Context, localAddress netip.Addr) (ManagementAccessStatus, error) {
	service.changeMu.Lock()
	defer service.changeMu.Unlock()
	if err := service.refresh(ctx); err != nil {
		return ManagementAccessStatus{}, err
	}
	return service.status(localAddress), nil
}

func (service *ManagementAccessService) status(localAddress netip.Addr) ManagementAccessStatus {
	service.mu.RLock()
	current := service.currentInterfaceLocked(localAddress)
	record := service.record
	interfaces := cloneInterfaces(service.interfaces)
	tlsMissing := append([]string(nil), service.tlsMissing...)
	service.mu.RUnlock()
	configuredNames := map[string]ManagementInterfaceBinding{}
	configuredAdapters := map[string]ManagementInterfaceBinding{}
	rollbackAdapters := map[string]ManagementInterfaceBinding{}
	for _, binding := range record.Configuration.Bindings {
		if binding.AdapterID != "" {
			configuredAdapters[binding.AdapterID] = binding
		} else {
			configuredNames[binding.LastKnownName] = binding
		}
	}
	if record.State == ManagementAccessPending || record.State == ManagementAccessFailed {
		for _, binding := range record.LastKnownGood.Bindings {
			if binding.AdapterID != "" {
				rollbackAdapters[canonicalAdapterID(binding.AdapterID)] = binding
			}
		}
	}
	status := ManagementAccessStatus{
		State: record.State, Port: record.Configuration.Port,
		Configured: effectiveManagementConfiguration(record.Configuration, interfaces).Interfaces,
		TLSNames:   append([]string(nil), record.Configuration.TLSNames...),
		TLSMissing: tlsMissing, ConfirmBy: record.ConfirmBy,
		Detail: record.Detail, Outcome: record.Outcome, CurrentInterface: current,
	}
	for _, candidate := range interfaces {
		binding, enabled := configuredAdapters[canonicalAdapterID(candidate.AdapterID)]
		if enabled {
			delete(configuredAdapters, binding.AdapterID)
		} else {
			binding, enabled = configuredNames[candidate.Name]
			if enabled {
				delete(configuredNames, candidate.Name)
			}
		}
		entry := ManagementAccessInterface{
			Name: candidate.Name, AdapterID: canonicalAdapterID(candidate.AdapterID), Kind: candidate.Kind, State: candidate.State,
			IPv4: append([]string(nil), candidate.IPv4...), IPv6: append([]string(nil), candidate.IPv6...),
			Enabled: enabled, Current: candidate.Name == current,
			Usable:        candidate.State == StateConnected && len(candidate.IPv4)+len(candidate.IPv6) > 0,
			RoleAvailable: true,
		}
		if rollback, held := rollbackAdapters[entry.AdapterID]; held && !entry.Enabled {
			entry.Role = AdapterRoleManagement
			entry.RoleDetail = "reserved for management access recovery"
			delete(rollbackAdapters, canonicalAdapterID(rollback.AdapterID))
		} else {
			service.addRoleMetadata(&entry)
			delete(rollbackAdapters, entry.AdapterID)
		}
		status.Interfaces = append(status.Interfaces, entry)
	}
	// A selected interface may be temporarily absent (for example nilo0 while
	// WireGuard reconnects). Keep the persistent choice visible without
	// inventing an address or claiming it is detected.
	for _, binding := range configuredAdapters {
		delete(rollbackAdapters, binding.AdapterID)
		nameReused := managementBindingNameAmbiguous(binding, record.Configuration.Bindings, interfaces)
		if !nameReused {
			status.Configured = append(status.Configured, binding.LastKnownName)
		}
		status.Interfaces = append(status.Interfaces, ManagementAccessInterface{
			Name: binding.LastKnownName, AdapterID: binding.AdapterID, Kind: KindOther, State: StateUnavailable,
			IPv4: []string{}, IPv6: []string{}, Enabled: true, Role: AdapterRoleManagement, RoleAvailable: !nameReused,
		})
	}
	for _, binding := range configuredNames {
		status.Configured = append(status.Configured, binding.LastKnownName)
		status.Interfaces = append(status.Interfaces, ManagementAccessInterface{
			Name: binding.LastKnownName, Kind: KindOther, State: StateUnavailable, Enabled: true,
			IPv4: []string{}, IPv6: []string{}, Role: AdapterRoleManagement, RoleAvailable: true,
		})
	}
	for _, binding := range rollbackAdapters {
		status.Interfaces = append(status.Interfaces, ManagementAccessInterface{
			Name: binding.LastKnownName, AdapterID: binding.AdapterID, Kind: KindOther, State: StateUnavailable,
			IPv4: []string{}, IPv6: []string{}, Role: AdapterRoleManagement, RoleAvailable: true,
			RoleDetail: "reserved for management access recovery",
		})
	}
	sort.Strings(status.Configured)
	sort.Slice(status.Interfaces, func(first, second int) bool {
		if status.Interfaces[first].Name == status.Interfaces[second].Name {
			return status.Interfaces[first].AdapterID < status.Interfaces[second].AdapterID
		}
		return status.Interfaces[first].Name < status.Interfaces[second].Name
	})
	for _, name := range record.Configuration.TLSNames {
		host := name
		kind := "local"
		if address, err := netip.ParseAddr(name); err == nil {
			host = address.String()
			kind = "vpn"
			if address.Is6() {
				host = "[" + host + "]"
			}
		}
		status.URLs = append(status.URLs, ManagementURL{URL: "https://" + host + ":" + strconv.Itoa(status.Port), Kind: kind})
	}
	return status
}

// A names-only edit cannot distinguish saved identities that share a name,
// whether the other adapter is currently observed or also unplugged.
func managementBindingNameAmbiguous(binding ManagementInterfaceBinding, saved []ManagementInterfaceBinding, interfaces []Interface) bool {
	for _, other := range saved {
		if other.LastKnownName == binding.LastKnownName && other.AdapterID != binding.AdapterID {
			return true
		}
	}
	for _, candidate := range interfaces {
		if candidate.Name == binding.LastKnownName && canonicalAdapterID(candidate.AdapterID) != binding.AdapterID {
			return true
		}
	}
	return false
}

func (service *ManagementAccessService) addRoleMetadata(entry *ManagementAccessInterface) {
	if entry.Enabled {
		entry.Role = AdapterRoleManagement
		entry.RoleAvailable = true
		entry.RoleDetail = "assigned to management access"
		return
	}
	if entry.AdapterID == "" {
		entry.RoleAvailable = true
		return
	}
	err := service.coordinator.Check([]AdapterRoleClaim{{
		AdapterID: entry.AdapterID, Interface: entry.Name, Role: AdapterRoleManagement,
	}})
	var conflict *AdapterRoleConflictError
	if !errors.As(err, &conflict) {
		entry.RoleAvailable = true
		return
	}
	entry.Role = conflict.ExistingRole
	entry.RoleAvailable = false
	entry.RoleDetail = conflict.Error()
	entry.ConflictingResourceID = conflict.ConflictingResourceID
	entry.ConflictingResourceName = conflict.ConflictingResourceName
}

func (service *ManagementAccessService) currentInterfaceLocked(address netip.Addr) string {
	if !address.IsValid() {
		return ""
	}
	return service.allowed[address.Unmap()]
}

// Update applies a new allowlist behind a confirmation window. If the caller
// removes its own arrival interface, ConfirmCurrent must be explicit.
func (service *ManagementAccessService) Update(ctx context.Context, interfaces []string, confirmCurrent bool, localAddress netip.Addr) (ManagementAccessStatus, error) {
	service.changeMu.Lock()
	defer service.changeMu.Unlock()
	if err := service.refresh(ctx); err != nil {
		return ManagementAccessStatus{}, err
	}
	service.mu.Lock()
	if service.record.State == ManagementAccessPending {
		service.mu.Unlock()
		return ManagementAccessStatus{}, ErrManagementAccessPending
	}
	next := service.record.Configuration
	next.Interfaces = append([]string(nil), interfaces...)
	next.Bindings = nil
	// Preserve an unplugged selection. An observed same-name adapter is an
	// explicit new choice and is resolved against its current physical identity.
	for _, name := range interfaces {
		binding := ManagementInterfaceBinding{LastKnownName: strings.TrimSpace(name)}
		observed := false
		for _, candidate := range service.interfaces {
			observed = observed || candidate.Name == binding.LastKnownName
		}
		if !observed {
			retained := false
			for _, saved := range service.record.Configuration.Bindings {
				if saved.LastKnownName == binding.LastKnownName {
					next.Bindings = append(next.Bindings, saved)
					retained = true
				}
			}
			if retained {
				continue
			}
		}
		next.Bindings = append(next.Bindings, binding)
	}
	// A name-only submission cannot deselect an absent identity whose name is
	// shared by another saved or observed adapter. Keep it until unambiguous.
	for _, saved := range service.record.Configuration.Bindings {
		present := false
		for _, candidate := range service.interfaces {
			present = present || canonicalAdapterID(candidate.AdapterID) == saved.AdapterID
		}
		if saved.AdapterID != "" && !present && managementBindingNameAmbiguous(saved, service.record.Configuration.Bindings, service.interfaces) {
			next.Bindings = append(next.Bindings, saved)
		}
	}
	next = resolveManagementConfiguration(next, service.interfaces)
	next = canonicalManagementConfiguration(next)
	if err := next.Validate(); err != nil {
		service.mu.Unlock()
		return ManagementAccessStatus{}, err
	}
	current := service.currentInterfaceLocked(localAddress)
	nextEffective := effectiveManagementConfiguration(next, service.interfaces)
	if current != "" && !containsString(nextEffective.Interfaces, current) && !confirmCurrent {
		service.mu.Unlock()
		return ManagementAccessStatus{}, fmt.Errorf("%w: %s", ErrCurrentAccessConfirmation, current)
	}
	if !service.hasUsableLocked(nextEffective.Interfaces) {
		service.mu.Unlock()
		return ManagementAccessStatus{}, ErrManagementAccessUnsafe
	}
	if sameManagementConfiguration(next, service.record.Configuration) {
		service.mu.Unlock()
		return service.status(localAddress), nil
	}
	previous := service.record.Configuration
	service.mu.Unlock()

	var record ManagementAccessRecord
	if err := service.coordinator.CheckAndRun(managementRoleClaims(next), func() error {
		if err := service.firewall.ApplyManagementAccess(ctx, nextEffective); err != nil {
			return service.publishFailedUpdate(context.WithoutCancel(ctx), previous, next, service.interfaces, err)
		}
		record = ManagementAccessRecord{
			Configuration: next, LastKnownGood: previous, State: ManagementAccessPending,
			ConfirmBy: service.now().UTC().Add(service.confirmWindow), UpdatedAt: service.now().UTC(),
		}
		if err := service.store.SaveManagementAccess(ctx, record); err != nil {
			return service.publishFailedUpdate(context.WithoutCancel(ctx), previous, next, service.interfaces, err)
		}
		service.mu.Lock()
		service.record = record
		service.rebuildAllowedLocked()
		service.mu.Unlock()
		return nil
	}); err != nil {
		var conflict *AdapterRoleConflictError
		if errors.As(err, &conflict) {
			return ManagementAccessStatus{}, err
		}
		return service.status(localAddress), err
	}
	return service.status(localAddress), nil
}

// publishFailedUpdate restores the previous firewall and publishes the exact
// claims that remain protected before the coordinator can admit another role.
// The caller holds the coordinator exclusion throughout this method.
func (service *ManagementAccessService) publishFailedUpdate(
	ctx context.Context,
	previous ManagementAccessConfiguration,
	candidate ManagementAccessConfiguration,
	interfaces []Interface,
	applyErr error,
) error {
	restoreErr := service.firewall.ApplyManagementAccess(ctx, effectiveManagementConfiguration(previous, interfaces))
	state := ManagementAccessRecovered
	detail := "the proposed management access failed and the last known-good configuration was restored"
	lastKnownGood := previous
	if restoreErr != nil {
		state = ManagementAccessFailed
		detail = "the proposed management access and its firewall recovery both failed"
		// The helper outcome is uncertain, so retain both physical role claims
		// until an administrator establishes and confirms a safe configuration.
		lastKnownGood = candidate
	}
	outcome := ManagementAccessOutcome{Code: ManagementOutcomeProposedRestored}
	if restoreErr != nil {
		outcome.Code = ManagementOutcomeRecoveryFailed
	}
	record := ManagementAccessRecord{
		Configuration: previous, LastKnownGood: lastKnownGood, State: state,
		Detail: detail, Outcome: outcome, UpdatedAt: service.now().UTC(),
	}
	persistErr := service.store.SaveManagementAccess(ctx, record)
	service.mu.Lock()
	service.interfaces = cloneInterfaces(interfaces)
	service.record = record
	service.rebuildAllowedLocked()
	service.mu.Unlock()
	if restoreErr != nil {
		return fmt.Errorf("apply management access: %v; restore last known-good firewall: %w", applyErr, restoreErr)
	}
	if persistErr != nil {
		return fmt.Errorf("apply management access: %v; persist recovered state: %w", applyErr, persistErr)
	}
	return fmt.Errorf("apply management access: %w", applyErr)
}

func (service *ManagementAccessService) hasUsableLocked(names []string) bool {
	return managementConfigurationHasUsableInterface(service.interfaces, names)
}

func managementConfigurationHasUsableInterface(interfaces []Interface, names []string) bool {
	for _, candidate := range interfaces {
		if containsString(names, candidate.Name) && candidate.State == StateConnected && len(candidate.IPv4)+len(candidate.IPv6) > 0 {
			return true
		}
	}
	return false
}

// Confirm keeps a pending configuration only when the confirmation itself
// arrived through one of its currently usable interfaces.
func (service *ManagementAccessService) Confirm(ctx context.Context, localAddress netip.Addr) (ManagementAccessStatus, error) {
	service.changeMu.Lock()
	defer service.changeMu.Unlock()
	if err := service.refresh(ctx); err != nil {
		return ManagementAccessStatus{}, err
	}
	service.mu.Lock()
	if service.record.State != ManagementAccessPending {
		service.mu.Unlock()
		return ManagementAccessStatus{}, ErrManagementAccessNotPending
	}
	if service.currentInterfaceLocked(localAddress) == "" {
		service.mu.Unlock()
		return ManagementAccessStatus{}, errors.New("confirmation must arrive through an enabled management interface")
	}
	record := service.record
	record.LastKnownGood = record.Configuration
	record.State = ManagementAccessApplied
	record.ConfirmBy = time.Time{}
	record.Detail = ""
	record.Outcome = ManagementAccessOutcome{}
	record.UpdatedAt = service.now().UTC()
	service.mu.Unlock()
	if err := service.store.SaveManagementAccess(ctx, record); err != nil {
		return ManagementAccessStatus{}, err
	}
	service.mu.Lock()
	service.record = record
	service.mu.Unlock()
	return service.status(localAddress), nil
}

// Recover restores an unconfirmed configuration. It is safe to call from the
// runtime timer, an explicit API action, and startup reconciliation.
func (service *ManagementAccessService) Recover(ctx context.Context, force bool) error {
	service.changeMu.Lock()
	defer service.changeMu.Unlock()
	if err := service.refresh(ctx); err != nil {
		return err
	}
	return service.recover(ctx, force)
}

func (service *ManagementAccessService) recover(ctx context.Context, force bool) error {
	service.mu.RLock()
	record := service.record
	service.mu.RUnlock()
	if record.State != ManagementAccessPending {
		return nil
	}
	if !force && service.now().UTC().Before(record.ConfirmBy) {
		return nil
	}
	if err := service.firewall.ApplyManagementAccess(ctx, effectiveManagementConfiguration(record.LastKnownGood, service.interfaces)); err != nil {
		service.mu.Lock()
		service.record.State = ManagementAccessFailed
		service.record.Detail = "automatic management access recovery failed"
		service.record.Outcome = ManagementAccessOutcome{Code: ManagementOutcomeAutomaticFailed}
		service.record.UpdatedAt = service.now().UTC()
		failed := service.record
		service.mu.Unlock()
		_ = service.store.SaveManagementAccess(context.WithoutCancel(ctx), failed)
		return err
	}
	record.Configuration = record.LastKnownGood
	record.State = ManagementAccessRecovered
	record.ConfirmBy = time.Time{}
	record.Detail = "the unconfirmed management access configuration was automatically restored"
	record.Outcome = ManagementAccessOutcome{Code: ManagementOutcomeAutomaticRestored}
	record.UpdatedAt = service.now().UTC()
	if err := service.store.SaveManagementAccess(ctx, record); err != nil {
		return err
	}
	service.mu.Lock()
	service.record = record
	service.rebuildAllowedLocked()
	service.mu.Unlock()
	return nil
}

// Reconcile refreshes addresses and expires a pending change. It is intended
// for the existing network maintenance loop.
func (service *ManagementAccessService) Reconcile(ctx context.Context) error {
	service.changeMu.Lock()
	defer service.changeMu.Unlock()
	if err := service.refresh(ctx); err != nil {
		return err
	}
	return service.recover(ctx, false)
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
