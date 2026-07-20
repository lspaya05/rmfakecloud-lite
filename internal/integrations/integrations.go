package integrations

import (
	"fmt"
	"time"

	"github.com/ddvk/rmfakecloud/internal/messages"
	"github.com/ddvk/rmfakecloud/internal/storage"
)

const (
	IcsProvider = "ics"
)

type IntegrationProvider interface{}

// CalendarIntegrationProvider abstracts calendar integrations
type CalendarIntegrationProvider interface {
	IntegrationProvider
	ListEvents(windowStart, windowEnd time.Time) (*messages.CalendarEventsResponse, error)
}

// getIntegrationProvider finds the integration provider for the user
func getIntegrationProvider(storer storage.UserStorer, uid, integrationid string) (IntegrationProvider, error) {
	usr, err := storer.GetUser(uid)
	if err != nil {
		return nil, err
	}
	for _, intg := range usr.Integrations {
		if intg.ID != integrationid {
			continue
		}
		switch intg.Provider {
		case IcsProvider:
			return newICS(intg), nil
		}
	}
	return nil, fmt.Errorf("integration not found or no implementation %s", integrationid)

}

func GetCalendarIntegrationProvider(storer storage.UserStorer, uid, integrationid string) (CalendarIntegrationProvider, error) {
	provider, err := getIntegrationProvider(storer, uid, integrationid)
	if err != nil {
		return nil, err
	}

	cip, ok := provider.(CalendarIntegrationProvider)
	if !ok {
		return nil, fmt.Errorf("provider %q is not a calendar provider", integrationid)
	}

	return cip, nil
}

// fix the name
func fixProviderName(n string) string {
	switch n {
	case IcsProvider:
		return "IcsCalendar"
	default:
		return n
	}
}

func ProviderType(n string) string {
	switch n {
	case IcsProvider:
		return "Calendar"
	default:
		return n
	}
}

// List lists the integrations
func List(userstorer storage.UserStorer, uid string) (*messages.IntegrationsResponse, error) {
	user, err := userstorer.GetUser(uid)
	if err != nil {
		return nil, err
	}

	res := &messages.IntegrationsResponse{}
	for _, userIntg := range user.Integrations {
		resIntg := messages.Integration{
			ID:           userIntg.ID,
			Name:         userIntg.Name,
			Provider:     fixProviderName(userIntg.Provider),
			ProviderType: ProviderType(userIntg.Provider),
			UserID:       uid,
		}

		res.Integrations = append(res.Integrations, resIntg)
	}

	return res, nil
}
