package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ddvk/rmfakecloud/internal/app/hub"
	"github.com/ddvk/rmfakecloud/internal/app/passcodestore"
	"github.com/ddvk/rmfakecloud/internal/config"
	"github.com/ddvk/rmfakecloud/internal/messages"
	"github.com/ddvk/rmfakecloud/internal/model"
	"github.com/ddvk/rmfakecloud/internal/screenshare"
	"github.com/ddvk/rmfakecloud/internal/storage/fs"
	"github.com/gin-gonic/gin"
)

const testAdminToken = "s3cr3t-admin-token"

func newAdminTestApp(t *testing.T) (*App, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		DataDir:       t.TempDir(),
		AdminAPIToken: testAdminToken,
		JWTSecretKey:  []byte("test-secret"),
	}
	store := fs.NewStorage(cfg)
	app := &App{
		cfg:           cfg,
		docStorer:     store,
		userStorer:    store,
		metaStorer:    store,
		blobStorer:    store,
		hub:           hub.NewHub(),
		passcodeStore: passcodestore.NewInMemory(),
		codeConnector: NewCodeConnector(),
		roomManager:   screenshare.NewRoomManager(),
	}

	router := gin.New()
	app.registerAdminRoutes(router)
	return app, router
}

func registerTestUser(t *testing.T, app *App, id string) *model.User {
	t.Helper()
	u, err := model.NewUser(id, "password123")
	if err != nil {
		t.Fatalf("NewUser: %v", err)
	}
	if err := app.userStorer.RegisterUser(u); err != nil {
		t.Fatalf("RegisterUser: %v", err)
	}
	return u
}

// doReq issues an authenticated (or unauthenticated) request against the router.
func doReq(t *testing.T, router *gin.Engine, method, path, token string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body != nil {
		r = httptest.NewRequest(method, path, bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	return w
}

func TestAdminAuth(t *testing.T) {
	app, router := newAdminTestApp(t)
	u := registerTestUser(t, app, "authuser")
	path := "/admin/users/" + u.ID + "/newcode"

	if w := doReq(t, router, http.MethodGet, path, "", nil); w.Code != http.StatusUnauthorized {
		t.Errorf("no token: expected 401, got %d", w.Code)
	}
	if w := doReq(t, router, http.MethodGet, path, "wrong-token", nil); w.Code != http.StatusUnauthorized {
		t.Errorf("wrong token: expected 401, got %d", w.Code)
	}
	if w := doReq(t, router, http.MethodGet, path, testAdminToken, nil); w.Code != http.StatusOK {
		t.Errorf("correct token: expected 200, got %d", w.Code)
	}
}

func TestAdminAuthTokenEdgeCases(t *testing.T) {
	app, router := newAdminTestApp(t)
	u := registerTestUser(t, app, "edgeuser")
	path := "/admin/users/" + u.ID + "/newcode"

	for _, tok := range []string{
		testAdminToken[:len(testAdminToken)-1], // prefix of the real token
		testAdminToken + "x",                   // real token with trailing garbage
		"Bearer " + testAdminToken,             // scheme accidentally duplicated in the token
	} {
		if w := doReq(t, router, http.MethodGet, path, tok, nil); w.Code != http.StatusUnauthorized {
			t.Errorf("token %q: expected 401, got %d", tok, w.Code)
		}
	}
}

func TestAdminNewCode(t *testing.T) {
	app, router := newAdminTestApp(t)
	u := registerTestUser(t, app, "codeuser")

	w := doReq(t, router, http.MethodGet, "/admin/users/"+u.ID+"/newcode", testAdminToken, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", w.Code, w.Body.String())
	}

	var code string
	if err := json.Unmarshal(w.Body.Bytes(), &code); err != nil {
		t.Fatalf("decode code: %v (body=%s)", err, w.Body.String())
	}
	if len(code) != 8 {
		t.Errorf("expected 8-char code, got %q", code)
	}

	uid, err := app.codeConnector.ConsumeCode(code)
	if err != nil {
		t.Fatalf("ConsumeCode: %v", err)
	}
	if uid != u.ID {
		t.Errorf("expected uid %q, got %q", u.ID, uid)
	}
}

func TestAdminPasscodeResetFlow(t *testing.T) {
	app, router := newAdminTestApp(t)
	u := registerTestUser(t, app, "pcuser")
	base := "/admin/users/" + u.ID + "/passcode/resets"

	mkReset := func(id string) {
		err := app.passcodeStore.Create(u.ID, messages.PasscodeReset{
			DeviceID:   "dev-1",
			DeviceName: "reMarkable 2",
			RequestID:  id,
			Created:    time.Now(),
			Expires:    time.Now().Add(time.Hour),
		})
		if err != nil {
			t.Fatalf("seed reset: %v", err)
		}
	}

	listResets := func() []messages.PasscodeReset {
		w := doReq(t, router, http.MethodGet, base, testAdminToken, nil)
		if w.Code != http.StatusOK {
			t.Fatalf("list: expected 200, got %d", w.Code)
		}
		var list []messages.PasscodeReset
		if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
			t.Fatalf("decode list: %v", err)
		}
		return list
	}

	// list → one pending
	mkReset("req-approve")
	if list := listResets(); len(list) != 1 || list[0].RequestID != "req-approve" {
		t.Fatalf("expected one pending reset, got %+v", list)
	}

	// approve → drops from pending list
	if w := doReq(t, router, http.MethodPost, base+"/req-approve/approve", testAdminToken, nil); w.Code != http.StatusOK {
		t.Fatalf("approve: expected 200, got %d", w.Code)
	}
	if list := listResets(); len(list) != 0 {
		t.Fatalf("expected no pending resets after approve, got %+v", list)
	}

	// dismiss → also drops from list
	mkReset("req-dismiss")
	if w := doReq(t, router, http.MethodDelete, base+"/req-dismiss", testAdminToken, nil); w.Code != http.StatusOK {
		t.Fatalf("dismiss: expected 200, got %d", w.Code)
	}
	if list := listResets(); len(list) != 0 {
		t.Fatalf("expected no pending resets after dismiss, got %+v", list)
	}

	// dismissing an unknown request → 404
	if w := doReq(t, router, http.MethodDelete, base+"/does-not-exist", testAdminToken, nil); w.Code != http.StatusNotFound {
		t.Errorf("dismiss unknown: expected 404, got %d", w.Code)
	}
}

func TestAdminIntegrationsCRUD(t *testing.T) {
	app, router := newAdminTestApp(t)
	u := registerTestUser(t, app, "intuser")
	base := "/admin/users/" + u.ID + "/integrations"

	// create
	body, _ := json.Marshal(model.IntegrationConfig{
		Provider: "ics",
		Name:     "my calendar",
		Address:  "http://localhost/cal.ics",
	})
	w := doReq(t, router, http.MethodPost, base, testAdminToken, body)
	if w.Code != http.StatusOK {
		t.Fatalf("create: expected 200, got %d (%s)", w.Code, w.Body.String())
	}
	var created model.IntegrationConfig
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created: %v", err)
	}
	if created.ID == "" {
		t.Fatal("created integration has no ID")
	}

	// list → one
	w = doReq(t, router, http.MethodGet, base, testAdminToken, nil)
	var list []model.IntegrationConfig
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(list) != 1 || list[0].ID != created.ID {
		t.Fatalf("expected one integration %q, got %+v", created.ID, list)
	}

	// get by id
	if w := doReq(t, router, http.MethodGet, base+"/"+created.ID, testAdminToken, nil); w.Code != http.StatusOK {
		t.Errorf("get: expected 200, got %d", w.Code)
	}

	// delete
	if w := doReq(t, router, http.MethodDelete, base+"/"+created.ID, testAdminToken, nil); w.Code != http.StatusAccepted {
		t.Errorf("delete: expected 202, got %d", w.Code)
	}

	// list → empty
	w = doReq(t, router, http.MethodGet, base, testAdminToken, nil)
	list = nil
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode list after delete: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected no integrations after delete, got %+v", list)
	}
}

func TestAdminIntegrationsRejectNonICS(t *testing.T) {
	app, router := newAdminTestApp(t)
	u := registerTestUser(t, app, "nonicsuser")
	base := "/admin/users/" + u.ID + "/integrations"

	// create with any non-ics provider → 400
	for _, provider := range []string{"dropbox", "localfs", "webdav", "ftp", ""} {
		body, _ := json.Marshal(model.IntegrationConfig{
			Provider: provider,
			Name:     "nope",
			Address:  "http://localhost/x",
		})
		if w := doReq(t, router, http.MethodPost, base, testAdminToken, body); w.Code != http.StatusBadRequest {
			t.Errorf("create provider %q: expected 400, got %d (%s)", provider, w.Code, w.Body.String())
		}
	}

	// update an existing ics integration to a non-ics provider → 400
	body, _ := json.Marshal(model.IntegrationConfig{
		Provider: "ics",
		Name:     "cal",
		Address:  "http://localhost/cal.ics",
	})
	w := doReq(t, router, http.MethodPost, base, testAdminToken, body)
	if w.Code != http.StatusOK {
		t.Fatalf("create ics: expected 200, got %d (%s)", w.Code, w.Body.String())
	}
	var created model.IntegrationConfig
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created: %v", err)
	}

	created.Provider = "dropbox"
	body, _ = json.Marshal(created)
	if w := doReq(t, router, http.MethodPut, base+"/"+created.ID, testAdminToken, body); w.Code != http.StatusBadRequest {
		t.Errorf("update to dropbox: expected 400, got %d (%s)", w.Code, w.Body.String())
	}
}

func TestAdminRoutesDisabledWithoutToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{DataDir: t.TempDir()} // no AdminAPIToken
	app := &App{cfg: cfg}
	router := gin.New()
	app.registerAdminRoutes(router)

	// With no token configured, the /admin group is never registered → 404.
	w := doReq(t, router, http.MethodGet, "/admin/users/x/newcode", testAdminToken, nil)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 when admin API disabled, got %d", w.Code)
	}
}
