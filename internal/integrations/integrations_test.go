package integrations

import (
	"fmt"
	"testing"

	"github.com/lspaya05/rmfakecloud-lite/internal/model"
)

type fakeUserStorer struct {
	user *model.User
}

func (f *fakeUserStorer) GetUsers() ([]*model.User, error) { return []*model.User{f.user}, nil }
func (f *fakeUserStorer) GetUser(id string) (*model.User, error) {
	if f.user != nil && f.user.ID == id {
		return f.user, nil
	}
	return nil, fmt.Errorf("user %q not found", id)
}
func (f *fakeUserStorer) RegisterUser(*model.User) error { return nil }
func (f *fakeUserStorer) UpdateUser(*model.User) error   { return nil }
func (f *fakeUserStorer) RemoveUser(string) error        { return nil }

func testStorer() *fakeUserStorer {
	return &fakeUserStorer{user: &model.User{
		ID: "u1",
		Integrations: []model.IntegrationConfig{
			{ID: "int-ics", Provider: IcsProvider, Name: "cal", Address: "http://localhost/cal.ics"},
			{ID: "int-legacy", Provider: "dropbox", Name: "old"},
		},
	}}
}

func TestGetCalendarIntegrationProvider(t *testing.T) {
	storer := testStorer()

	if _, err := GetCalendarIntegrationProvider(storer, "u1", "int-ics"); err != nil {
		t.Errorf("ics integration: unexpected error %v", err)
	}
	// a leftover legacy provider in the profile has no implementation anymore
	if _, err := GetCalendarIntegrationProvider(storer, "u1", "int-legacy"); err == nil {
		t.Error("legacy dropbox integration: expected error, got provider")
	}
	if _, err := GetCalendarIntegrationProvider(storer, "u1", "missing"); err == nil {
		t.Error("unknown integration id: expected error")
	}
	if _, err := GetCalendarIntegrationProvider(storer, "nobody", "int-ics"); err == nil {
		t.Error("unknown user: expected error")
	}
}

func TestListMapsProviderNames(t *testing.T) {
	res, err := List(testStorer(), "u1")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(res.Integrations) != 2 {
		t.Fatalf("expected 2 integrations, got %d", len(res.Integrations))
	}

	ics := res.Integrations[0]
	if ics.Provider != "IcsCalendar" || ics.ProviderType != "Calendar" {
		t.Errorf("ics mapping: expected IcsCalendar/Calendar, got %s/%s", ics.Provider, ics.ProviderType)
	}
	if ics.UserID != "u1" || ics.ID != "int-ics" || ics.Name != "cal" {
		t.Errorf("ics fields lost: %+v", ics)
	}

	// unknown providers pass through unchanged (still listed so the user can see/remove them)
	legacy := res.Integrations[1]
	if legacy.Provider != "dropbox" || legacy.ProviderType != "dropbox" {
		t.Errorf("legacy mapping: expected passthrough, got %s/%s", legacy.Provider, legacy.ProviderType)
	}
}

func TestProviderNameMapping(t *testing.T) {
	if got := fixProviderName(IcsProvider); got != "IcsCalendar" {
		t.Errorf("fixProviderName(ics): got %q", got)
	}
	if got := fixProviderName("other"); got != "other" {
		t.Errorf("fixProviderName(other): got %q", got)
	}
	if got := ProviderType(IcsProvider); got != "Calendar" {
		t.Errorf("ProviderType(ics): got %q", got)
	}
	if got := ProviderType("other"); got != "other" {
		t.Errorf("ProviderType(other): got %q", got)
	}
}
