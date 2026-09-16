package appliance

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"nilo.local/nilo/internal/network"
)

var (
	ErrInvalidFriendlyName     = errors.New("invalid friendly appliance name")
	ErrFriendlyNameConflict    = errors.New("friendly appliance name conflicts on the local link")
	ErrFriendlyNameUnavailable = errors.New("friendly appliance name service unavailable")
)

// FriendlyNameRepository persists only the optional discovery alias.
type FriendlyNameRepository interface {
	SetFriendlyName(context.Context, string) (Identity, error)
}

// CertificateNameManager activates a certificate covering exactly the
// supplied local labels before any corresponding publication is visible.
type CertificateNameManager interface {
	ActivateNames(context.Context, []string) error
}

// FriendlyNamePublisher atomically replaces only the friendly alias groups.
type FriendlyNamePublisher interface {
	ReplaceFriendlyName(context.Context, string) error
}

// NamingService serializes the cross-store friendly-name saga. The permanent
// serial publication is outside this service and cannot be withdrawn by it.
type NamingService struct {
	mu           sync.Mutex
	serial       string
	current      string
	repository   FriendlyNameRepository
	certificates CertificateNameManager
	publisher    FriendlyNamePublisher
}

// NewNamingService validates the durable starting state and dependencies.
func NewNamingService(
	serial, current string,
	repository FriendlyNameRepository,
	certificates CertificateNameManager,
	publisher FriendlyNamePublisher,
) (*NamingService, error) {
	if !validPermanentName(serial) {
		return nil, errors.New("valid permanent appliance name is required")
	}
	if err := ValidateFriendlyName(current); err != nil || current == serial {
		return nil, errors.New("valid distinct friendly appliance name is required")
	}
	if repository == nil || certificates == nil || publisher == nil {
		return nil, errors.New("naming service dependencies are required")
	}
	return &NamingService{
		serial: serial, current: current, repository: repository,
		certificates: certificates, publisher: publisher,
	}, nil
}

// SetFriendlyName covers, publishes and persists a candidate, rolling visible
// and cryptographic state back to the previous durable value on any failure.
func (service *NamingService) SetFriendlyName(ctx context.Context, candidate string) (Identity, error) {
	if err := ValidateFriendlyName(candidate); err != nil || candidate == service.serial {
		return Identity{}, ErrInvalidFriendlyName
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	if candidate == service.current {
		return Identity{FriendlyName: service.current}, nil
	}
	previous := service.current
	prepareNames := service.certificateNames(previous, candidate)
	if err := service.certificates.ActivateNames(ctx, prepareNames); err != nil {
		return Identity{}, fmt.Errorf("%w: prepare certificate", ErrFriendlyNameUnavailable)
	}
	if candidate != "" {
		if err := service.publisher.ReplaceFriendlyName(ctx, candidate); err != nil {
			rollback := context.WithoutCancel(ctx)
			publishErr := service.publisher.ReplaceFriendlyName(rollback, previous)
			certificateErr := service.certificates.ActivateNames(rollback, service.certificateNames(previous))
			kind := ErrFriendlyNameUnavailable
			if errors.Is(err, network.ErrMDNSCollision) {
				kind = ErrFriendlyNameConflict
			}
			return Identity{}, errors.Join(fmt.Errorf("%w: publish candidate", kind), publishErr, certificateErr)
		}
	}
	identity, err := service.repository.SetFriendlyName(ctx, candidate)
	if err != nil {
		rollback := context.WithoutCancel(ctx)
		publishErr := service.publisher.ReplaceFriendlyName(rollback, previous)
		certificateErr := service.certificates.ActivateNames(rollback, service.certificateNames(previous))
		return Identity{}, errors.Join(fmt.Errorf("%w: persist friendly appliance name", ErrFriendlyNameUnavailable), publishErr, certificateErr)
	}
	if candidate == "" {
		if err := service.publisher.ReplaceFriendlyName(ctx, ""); err != nil {
			rollback := context.WithoutCancel(ctx)
			_, persistErr := service.repository.SetFriendlyName(rollback, previous)
			publishErr := service.publisher.ReplaceFriendlyName(rollback, previous)
			certificateErr := service.certificates.ActivateNames(rollback, service.certificateNames(previous))
			return Identity{}, errors.Join(fmt.Errorf("%w: withdraw friendly appliance name", ErrFriendlyNameUnavailable), persistErr, publishErr, certificateErr)
		}
	}
	if err := service.certificates.ActivateNames(ctx, service.certificateNames(candidate)); err != nil {
		rollback := context.WithoutCancel(ctx)
		publishErr := service.publisher.ReplaceFriendlyName(rollback, previous)
		_, persistErr := service.repository.SetFriendlyName(rollback, previous)
		certificateErr := service.certificates.ActivateNames(rollback, service.certificateNames(previous))
		return Identity{}, errors.Join(fmt.Errorf("%w: finalize friendly-name certificate", ErrFriendlyNameUnavailable), publishErr, persistErr, certificateErr)
	}
	service.current = candidate
	return identity, nil
}

func (service *NamingService) certificateNames(values ...string) []string {
	return uniqueNames(append([]string{service.serial, "nilo"}, values...)...)
}

func uniqueNames(values ...string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}
