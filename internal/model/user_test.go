package model

import "testing"

// A .userprofile written before the lite refactor may contain integration
// fields for providers that no longer exist (dropbox/ftp/webdav/localfs).
// yaml.v3 ignores unknown keys, so such profiles must still deserialize and
// keep the fields the ICS provider uses.
func TestDeserializeUserLegacyIntegrationFields(t *testing.T) {
	legacy := []byte(`
id: legacyuser
email: legacyuser
emailverified: true
password: $argon2id$v=19$m=3072,t=5,p=4$c2FsdA$aGFzaA
sync15: true
integrations:
    - id: int-1
      provider: ics
      name: my calendar
      address: http://localhost/cal.ics
      insecure: true
      username: olduser
      password: oldpass
      path: /old/path
      accesstoken: old-token
      endpoint: https://old.example.com
      activetransfers: 2
    - id: int-2
      provider: dropbox
      name: old dropbox
      accesstoken: dropbox-token
`)

	u, err := DeserializeUser(legacy)
	if err != nil {
		t.Fatalf("DeserializeUser: %v", err)
	}

	if u.ID != "legacyuser" {
		t.Errorf("expected ID legacyuser, got %q", u.ID)
	}
	if len(u.Integrations) != 2 {
		t.Fatalf("expected 2 integrations, got %d", len(u.Integrations))
	}

	ics := u.Integrations[0]
	if ics.ID != "int-1" || ics.Provider != "ics" || ics.Name != "my calendar" {
		t.Errorf("ics integration fields lost: %+v", ics)
	}
	if ics.Address != "http://localhost/cal.ics" {
		t.Errorf("expected address kept, got %q", ics.Address)
	}
	if !ics.Insecure {
		t.Error("expected insecure=true kept")
	}

	// round-trip: re-serializing must not fail even though unknown keys were dropped
	if _, err := u.Serialize(); err != nil {
		t.Fatalf("Serialize after legacy load: %v", err)
	}
}
