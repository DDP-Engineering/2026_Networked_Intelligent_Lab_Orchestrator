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

// ManagementAccessConfiguration is the stable listener identity. Interface
// names, not their DHCP addresses, are the access-control boundary.
type ManagementAccessConfiguration struct {
	Interfaces []string
	Port       int
	TLSNames   []string
}

// Validate rejects configurations that could become a wildcard listener
// without an explicit, meaningful allowlist.
func (configuration ManagementAccessConfiguration) Validate() error {
	if configuration.Port < 1 || configuration.Port > 65535 {
		return errors.New("management port must be between 1 and 65535")
	}
	if len(configuration.Interfaces) == 0 {
		return ErrManagementAccessRequired
	}
	seen := map[string]bool{}
	for _, name := range configuration.Interfaces {
		if !managementInterfaceName.MatchString(name) || name == "lo" {
			return fmt.Errorf("invalid management interface %q", name)
		}
		if seen[name] {
			return fmt.Errorf("duplicate management interface %q", name)
		}
		seen[name] = true
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
	configuration.Interfaces = append([]string(nil), configuration.Interfaces...)
	configuration.TLSNames = append([]string(nil), configuration.TLSNames...)
	sort.Strings(configuration.Interfaces)
	sort.Strings(configuration.TLSNames)
	return configuration
}

func sameManagementConfiguration(first, second ManagementAccessConfiguration) bool {
	first = canonicalManagementConfiguration(first)
	second = canonicalManagementConfiguration(second)
	return first.Port == second.Port && strings.Join(first.Interfaces, "\x00") == strings.Join(second.Interfaces, "\x00") &&
		strings.Join(first.TLSNames, "\x00") == strings.Join(second.TLSNames, "\x00")
}

// ManagementAccessRecord is the one durable management-listener decision.
// LastKnownGood is never empty after initial configuration.
type ManagementAccessRecord struct {
	Configuration ManagementAccessConfiguration
	LastKnownGood ManagementAccessConfiguration
	State         ManagementAccessState
	ConfirmBy     time.Time
	Detail        string
	UpdatedAt     time.Time
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
	Name    string   `json:"name"`
	Kind    Kind     `json:"kind"`
	State   State    `json:"state"`
	IPv4    []string `json:"ipv4"`
	IPv6    []string `json:"ipv6"`
	Enabled bool     `json:"enabled"`
	Current bool     `json:"current"`
	Usable  bool     `json:"usable"`
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
	record, present, err := options.Store.LoadManagementAccess(ctx)
	if err != nil {
		return nil, fmt.Errorf("load management access: %w", err)
	}
	if record.State == ManagementAccessPending {
		if err := record.LastKnownGood.Validate(); err != nil {
			return nil, fmt.Errorf("pending management access has no valid recovery configuration: %w", err)
		}
		record.Configuration = record.LastKnownGood
		record.State = ManagementAccessRecovered
		record.ConfirmBy = time.Time{}
		record.Detail = "an unconfirmed management access change was restored during startup"
		record.UpdatedAt = options.Now().UTC()
		if err := options.Store.SaveManagementAccess(ctx, record); err != nil {
			return nil, fmt.Errorf("record startup management recovery: %w", err)
		}
	}
	if options.Initial != nil {
		configuration := canonicalManagementConfiguration(*options.Initial)
		if err := configuration.Validate(); err != nil {
			return nil, err
		}
		interfaces, err := options.Provider.Interfaces(ctx)
		if err != nil {
			return nil, fmt.Errorf("observe initial management interfaces: %w", err)
		}
		if !managementConfigurationHasUsableInterface(interfaces, configuration.Interfaces) {
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
			}
		}
		if err := options.Store.SaveManagementAccess(ctx, record); err != nil {
			return nil, fmt.Errorf("save initial management access: %w", err)
		}
		present = true
	}
	if !present {
		return nil, errors.New("no persistent management access configuration; specify --management-interface at the appliance console")
	}
	if err := record.Configuration.Validate(); err != nil {
		return nil, fmt.Errorf("invalid saved management access: %w", err)
	}
	if err := options.Firewall.ApplyManagementAccess(ctx, record.Configuration); err != nil {
		return nil, fmt.Errorf("apply management firewall: %w", err)
	}
	service := &ManagementAccessService{
		provider: options.Provider, store: options.Store, firewall: options.Firewall,
		now: options.Now, confirmWindow: options.ConfirmWindow, record: record,
		allowed: map[netip.Addr]string{}, tlsMissing: append([]string(nil), options.TLSMissing...),
	}
	if err := service.Refresh(ctx); err != nil {
		return nil, err
	}
	return service, nil
}

// Refresh updates the address cache used by the pre-HTTP socket gate. It does
// not alter the allowlist, so DHCP renewal and interface return need no restart
// or configuration write.
func (service *ManagementAccessService) Refresh(ctx context.Context) error {
	interfaces, err := service.provider.Interfaces(ctx)
	if err != nil {
		return fmt.Errorf("refresh management interfaces: %w", err)
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	service.interfaces = cloneInterfaces(interfaces)
	service.rebuildAllowedLocked()
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
	configured := map[string]bool{}
	for _, name := range service.record.Configuration.Interfaces {
		configured[name] = true
	}
	allowed := map[netip.Addr]string{}
	for _, candidate := range service.interfaces {
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
	service.allowed = allowed
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
	return append([]string(nil), service.record.Configuration.Interfaces...)
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
	if err := service.Refresh(ctx); err != nil {
		return ManagementAccessStatus{}, err
	}
	service.mu.RLock()
	defer service.mu.RUnlock()
	current := service.currentInterfaceLocked(localAddress)
	configured := map[string]bool{}
	for _, name := range service.record.Configuration.Interfaces {
		configured[name] = true
	}
	status := ManagementAccessStatus{
		State: service.record.State, Port: service.record.Configuration.Port,
		Configured: append([]string(nil), service.record.Configuration.Interfaces...),
		TLSNames:   append([]string(nil), service.record.Configuration.TLSNames...),
		TLSMissing: append([]string(nil), service.tlsMissing...), ConfirmBy: service.record.ConfirmBy,
		Detail: service.record.Detail, CurrentInterface: current,
	}
	for _, candidate := range service.interfaces {
		enabled := configured[candidate.Name]
		delete(configured, candidate.Name)
		status.Interfaces = append(status.Interfaces, ManagementAccessInterface{
			Name: candidate.Name, Kind: candidate.Kind, State: candidate.State,
			IPv4: append([]string(nil), candidate.IPv4...), IPv6: append([]string(nil), candidate.IPv6...),
			Enabled: enabled, Current: candidate.Name == current,
			Usable: candidate.State == StateConnected && len(candidate.IPv4)+len(candidate.IPv6) > 0,
		})
	}
	// A selected interface may be temporarily absent (for example nilo0 while
	// WireGuard reconnects). Keep the persistent choice visible without
	// inventing an address or claiming it is detected.
	for name := range configured {
		status.Interfaces = append(status.Interfaces, ManagementAccessInterface{
			Name: name, Kind: KindOther, State: StateUnavailable, Enabled: true,
			IPv4: []string{}, IPv6: []string{},
		})
	}
	sort.Slice(status.Interfaces, func(first, second int) bool {
		return status.Interfaces[first].Name < status.Interfaces[second].Name
	})
	for _, name := range service.record.Configuration.TLSNames {
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
	return status, nil
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
	if err := service.Refresh(ctx); err != nil {
		return ManagementAccessStatus{}, err
	}
	service.mu.Lock()
	if service.record.State == ManagementAccessPending {
		service.mu.Unlock()
		return ManagementAccessStatus{}, ErrManagementAccessPending
	}
	next := service.record.Configuration
	next.Interfaces = append([]string(nil), interfaces...)
	next = canonicalManagementConfiguration(next)
	if err := next.Validate(); err != nil {
		service.mu.Unlock()
		return ManagementAccessStatus{}, err
	}
	current := service.currentInterfaceLocked(localAddress)
	if current != "" && !containsString(next.Interfaces, current) && !confirmCurrent {
		service.mu.Unlock()
		return ManagementAccessStatus{}, fmt.Errorf("%w: %s", ErrCurrentAccessConfirmation, current)
	}
	if !service.hasUsableLocked(next.Interfaces) {
		service.mu.Unlock()
		return ManagementAccessStatus{}, ErrManagementAccessUnsafe
	}
	if sameManagementConfiguration(next, service.record.Configuration) {
		service.mu.Unlock()
		return service.Status(ctx, localAddress)
	}
	previous := service.record.Configuration
	service.mu.Unlock()

	if err := service.firewall.ApplyManagementAccess(ctx, next); err != nil {
		return service.failedUpdate(ctx, previous, err, localAddress)
	}
	record := ManagementAccessRecord{
		Configuration: next, LastKnownGood: previous, State: ManagementAccessPending,
		ConfirmBy: service.now().UTC().Add(service.confirmWindow), UpdatedAt: service.now().UTC(),
	}
	if err := service.store.SaveManagementAccess(ctx, record); err != nil {
		return service.failedUpdate(ctx, previous, err, localAddress)
	}
	service.mu.Lock()
	service.record = record
	service.rebuildAllowedLocked()
	service.mu.Unlock()
	return service.Status(ctx, localAddress)
}

func (service *ManagementAccessService) failedUpdate(ctx context.Context, previous ManagementAccessConfiguration, applyErr error, localAddress netip.Addr) (ManagementAccessStatus, error) {
	restoreErr := service.firewall.ApplyManagementAccess(ctx, previous)
	state := ManagementAccessRecovered
	detail := "the proposed management access failed and the last known-good configuration was restored"
	if restoreErr != nil {
		state = ManagementAccessFailed
		detail = "the proposed management access and its firewall recovery both failed"
	}
	record := ManagementAccessRecord{
		Configuration: previous, LastKnownGood: previous, State: state,
		Detail: detail, UpdatedAt: service.now().UTC(),
	}
	_ = service.store.SaveManagementAccess(context.WithoutCancel(ctx), record)
	service.mu.Lock()
	service.record = record
	service.rebuildAllowedLocked()
	service.mu.Unlock()
	if restoreErr != nil {
		return ManagementAccessStatus{}, fmt.Errorf("apply management access: %v; restore last known-good firewall: %w", applyErr, restoreErr)
	}
	status, _ := service.Status(context.WithoutCancel(ctx), localAddress)
	return status, fmt.Errorf("apply management access: %w", applyErr)
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
	if err := service.Refresh(ctx); err != nil {
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
	record.UpdatedAt = service.now().UTC()
	service.mu.Unlock()
	if err := service.store.SaveManagementAccess(ctx, record); err != nil {
		return ManagementAccessStatus{}, err
	}
	service.mu.Lock()
	service.record = record
	service.mu.Unlock()
	return service.Status(ctx, localAddress)
}

// Recover restores an unconfirmed configuration. It is safe to call from the
// runtime timer, an explicit API action, and startup reconciliation.
func (service *ManagementAccessService) Recover(ctx context.Context, force bool) error {
	service.changeMu.Lock()
	defer service.changeMu.Unlock()
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
	if err := service.firewall.ApplyManagementAccess(ctx, record.LastKnownGood); err != nil {
		service.mu.Lock()
		service.record.State = ManagementAccessFailed
		service.record.Detail = "automatic management access recovery failed"
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
	if err := service.Refresh(ctx); err != nil {
		return err
	}
	service.changeMu.Lock()
	defer service.changeMu.Unlock()
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
