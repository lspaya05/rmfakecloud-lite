package app

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"time"

	"github.com/lspaya05/rmfakecloud-lite/internal/common"
	"github.com/lspaya05/rmfakecloud-lite/internal/integrations"
	"github.com/lspaya05/rmfakecloud-lite/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
)

// Headless admin JSON API. All routes live under /admin and are protected by a
// static bearer token (RM_ADMIN_API_TOKEN). The handlers are near-verbatim
// relocations of the former web-UI handlers so an external tool (curl or a local
// microservice) can drive pairing, passcode resets, calendar integrations and
// screenshare viewer signaling without the React frontend.

const (
	adminLog    = "[admin] "
	useridParam = "userid"
	intIDParam  = "intid"
)

// adminAuthMiddleware validates the static bearer token in constant time.
func (app *App) adminAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		token, err := common.GetToken(c)
		if err != nil || app.cfg.AdminAPIToken == "" ||
			subtle.ConstantTimeCompare([]byte(token), []byte(app.cfg.AdminAPIToken)) != 1 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		c.Next()
	}
}

// adminUserParam sets the sanitized :userid into the context so the relocated
// handlers keep reading the uid via userID(c), exactly as in the UI originals.
func adminUserParam() gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := common.SanitizeUid(c.Param(useridParam))
		c.Set(userIDKey, uid)
		c.Next()
	}
}

func (app *App) registerAdminRoutes(router *gin.Engine) {
	if app.cfg.AdminAPIToken == "" {
		log.Warn(adminLog, "admin API disabled: set ", "RM_ADMIN_API_TOKEN", " to enable the /admin endpoints")
		return
	}
	log.Info(adminLog, "admin API enabled under /admin")

	admin := router.Group("/admin")
	admin.Use(app.adminAuthMiddleware())

	users := admin.Group("/users/:" + useridParam)
	users.Use(adminUserParam())

	users.GET("/newcode", app.adminNewCode)

	users.GET("/passcode/resets", app.adminListPasscodeResets)
	users.POST("/passcode/resets/:uuid/approve", app.adminApprovePasscodeReset)
	users.DELETE("/passcode/resets/:uuid", app.adminDismissPasscodeReset)

	users.GET("/integrations", app.adminListIntegrations)
	users.POST("/integrations", app.adminCreateIntegration)
	users.GET("/integrations/:"+intIDParam, app.adminGetIntegration)
	users.PUT("/integrations/:"+intIDParam, app.adminUpdateIntegration)
	users.DELETE("/integrations/:"+intIDParam, app.adminDeleteIntegration)

	users.GET("/screenshare/room", app.adminScreenshareJoinActive)
	users.GET("/screenshare/room/:roomId", app.adminScreenshareGetRoom)
	users.GET("/screenshare/offer", app.adminScreenshareGetOffer)
	users.POST("/screenshare/room/:roomId/answer", app.adminScreenshareSendAnswer)
	users.DELETE("/screenshare/room/:roomId", app.adminScreenshareDeleteRoom)
}

// --- pairing ---

func (app *App) adminNewCode(c *gin.Context) {
	uid := userID(c)

	user, err := app.userStorer.GetUser(uid)
	if err != nil {
		log.Error(adminLog, "unable to find user: ", err)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	code, err := app.codeConnector.NewCode(user.ID)
	if err != nil {
		log.Error(adminLog, "unable to generate new device code: ", err)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "unable to generate new code"})
		return
	}

	c.JSON(http.StatusOK, code)
}

// --- passcode resets ---

func (app *App) adminListPasscodeResets(c *gin.Context) {
	uid := userID(c)
	list := app.passcodeStore.ListForUser(uid)
	c.JSON(http.StatusOK, list)
}

func (app *App) adminApprovePasscodeReset(c *gin.Context) {
	uid := userID(c)
	requestID := c.Param("uuid")
	if requestID == "" {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	reset, err := app.passcodeStore.Approve(uid, requestID)
	if err != nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}

	app.hub.NotifyPasscodeReset(uid, reset.DeviceID, reset.DeviceName, reset.RequestID)
	log.Infof("%sapproved reset request %s for device %s", adminLog, reset.RequestID, reset.DeviceID)
	c.Status(http.StatusOK)
}

func (app *App) adminDismissPasscodeReset(c *gin.Context) {
	uid := userID(c)
	requestID := c.Param("uuid")
	if requestID == "" {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	if err := app.passcodeStore.Delete(uid, requestID); err != nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	log.Infof("%sdismissed reset request %s", adminLog, requestID)
	c.Status(http.StatusOK)
}

// --- calendar (ICS) integrations ---
// Provider handling is copied as-is from the UI originals; it is trimmed to
// ICS-only in a later phase.

func (app *App) adminListIntegrations(c *gin.Context) {
	uid := userID(c)

	user, err := app.userStorer.GetUser(uid)
	if err != nil {
		log.Error(adminLog, err)
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	c.JSON(http.StatusOK, user.Integrations)
}

func (app *App) adminCreateIntegration(c *gin.Context) {
	int := model.IntegrationConfig{}
	if err := c.ShouldBindJSON(&int); err != nil {
		log.Error(adminLog, err)
		badReq(c, err.Error())
		return
	}

	if int.Provider != integrations.IcsProvider {
		badReq(c, "only the \"ics\" provider is supported")
		return
	}

	uid := userID(c)

	user, err := app.userStorer.GetUser(uid)
	if err != nil {
		log.Error(adminLog, err)
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	int.ID = uuid.NewString()
	user.Integrations = append(user.Integrations, int)

	if err = app.userStorer.UpdateUser(user); err != nil {
		log.Error(adminLog, "error updating user", err)
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	c.JSON(http.StatusOK, int)
}

func (app *App) adminGetIntegration(c *gin.Context) {
	uid := userID(c)
	intid := common.ParamS(intIDParam, c)

	user, err := app.userStorer.GetUser(uid)
	if err != nil {
		log.Error(adminLog, err)
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	for _, integration := range user.Integrations {
		if integration.ID == intid {
			c.JSON(http.StatusOK, integration)
			return
		}
	}

	c.AbortWithStatus(http.StatusNotFound)
}

func (app *App) adminUpdateIntegration(c *gin.Context) {
	int := model.IntegrationConfig{}
	if err := c.ShouldBindJSON(&int); err != nil {
		log.Error(adminLog, err)
		badReq(c, err.Error())
		return
	}

	if int.Provider != integrations.IcsProvider {
		badReq(c, "only the \"ics\" provider is supported")
		return
	}

	uid := userID(c)
	intid := common.ParamS(intIDParam, c)

	user, err := app.userStorer.GetUser(uid)
	if err != nil {
		log.Error(adminLog, err)
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	for idx, integration := range user.Integrations {
		if integration.ID == intid {
			int.ID = integration.ID
			user.Integrations[idx] = int

			if err = app.userStorer.UpdateUser(user); err != nil {
				log.Error(adminLog, "error updating user", err)
				c.AbortWithStatus(http.StatusInternalServerError)
				return
			}

			c.JSON(http.StatusOK, int)
			return
		}
	}

	c.AbortWithStatus(http.StatusNotFound)
}

func (app *App) adminDeleteIntegration(c *gin.Context) {
	uid := userID(c)
	intid := common.ParamS(intIDParam, c)

	user, err := app.userStorer.GetUser(uid)
	if err != nil {
		log.Error(adminLog, err)
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	for idx, integration := range user.Integrations {
		if integration.ID == intid {
			user.Integrations = append(user.Integrations[:idx], user.Integrations[idx+1:]...)

			if err = app.userStorer.UpdateUser(user); err != nil {
				log.Error(adminLog, "error updating user", err)
				c.AbortWithStatus(http.StatusInternalServerError)
				return
			}

			c.Status(http.StatusAccepted)
			return
		}
	}

	c.AbortWithStatus(http.StatusNotFound)
}

// --- screenshare viewer signaling ---
// The viewer never connects to MQTT directly; the server relays signaling on its
// behalf. clientId (query param) replaces the former browser cookie; when empty
// the server generates one and returns it so the caller can reuse it.

func (app *App) adminScreenshareJoinActive(c *gin.Context) {
	uid := userID(c)

	roomID := app.roomManager.FindActiveRoom(uid)
	if roomID == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "no active room"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"roomId":     roomID,
		"clients":    app.roomManager.GetClients(roomID),
		"iceServers": app.cfg.ICEServers,
	})
}

func (app *App) adminScreenshareGetRoom(c *gin.Context) {
	roomID := c.Param("roomId")

	room := app.roomManager.GetRoom(roomID)
	if room == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "room not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"roomId":    room.RoomID,
		"createdAt": room.CreatedAt.Format(time.RFC3339Nano),
		"clients":   app.roomManager.GetClients(roomID),
	})
}

func (app *App) adminScreenshareGetOffer(c *gin.Context) {
	uid := userID(c)
	clientID := c.Query("clientId")
	if clientID == "" {
		clientID = uuid.NewString()
	}

	roomID := app.roomManager.FindActiveRoom(uid)
	if roomID == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "no active room"})
		return
	}

	app.roomManager.AddBroadcast(roomID, clientID, json.RawMessage(`{"type":"request-offer","clientId":"`+clientID+`"}`))

	var inner map[string]interface{}
	json.Unmarshal([]byte(`{"type":"request-offer","clientId":"`+clientID+`","sourceDeviceID":"`+clientID+`"}`), &inner)
	app.hub.NotifyScreenshare(uid, clientID, inner)

	if app.mqttBroker != nil && app.mqttBroker.HasConnectedClient(uid) {
		clients := app.roomManager.GetClients(roomID)
		for _, cl := range clients {
			if cl.IsOwner {
				mqttMsg, _ := json.Marshal(map[string]interface{}{
					"type":     "broadcast",
					"clientId": clientID,
					"payload":  json.RawMessage(`{"type":"request-offer","clientId":"` + clientID + `"}`),
				})
				app.mqttBroker.PublishSignaling(uid, cl.ClientID, mqttMsg)
				break
			}
		}
	}

	msgs := app.roomManager.WaitForMessages(roomID, 1, 30*time.Second)
	if msgs == nil {
		c.JSON(http.StatusGatewayTimeout, gin.H{"error": "timeout waiting for offer"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"roomId":     roomID,
		"clientId":   clientID,
		"messages":   msgs,
		"iceServers": app.cfg.ICEServers,
	})
}

func (app *App) adminScreenshareSendAnswer(c *gin.Context) {
	roomID := c.Param("roomId")
	clientID := c.Query("clientId")
	if clientID == "" {
		clientID = uuid.NewString()
	}
	uid := userID(c)

	if !app.roomManager.RoomExists(roomID) {
		c.JSON(http.StatusNotFound, gin.H{"error": "room not found"})
		return
	}

	var msg struct {
		Payload        json.RawMessage `json:"payload"`
		TargetClientID string          `json:"targetClientId"`
	}
	if err := c.ShouldBindJSON(&msg); err != nil {
		badReq(c, "invalid body")
		return
	}

	app.roomManager.AddDirect(roomID, clientID, msg.TargetClientID, msg.Payload)

	var inner map[string]interface{}
	json.Unmarshal(msg.Payload, &inner)
	inner["sourceDeviceID"] = clientID
	app.hub.NotifyScreenshare(uid, clientID, inner)

	if app.mqttBroker != nil && app.mqttBroker.HasConnectedClient(uid) {
		mqttMsg, _ := json.Marshal(map[string]interface{}{
			"type":     "direct",
			"clientId": clientID,
			"payload":  json.RawMessage(msg.Payload),
		})
		app.mqttBroker.PublishSignaling(uid, msg.TargetClientID, mqttMsg)
	}

	c.Status(http.StatusAccepted)
}

func (app *App) adminScreenshareDeleteRoom(c *gin.Context) {
	uid := userID(c)
	app.roomManager.DeleteAllForUser(uid)
	c.Status(http.StatusNoContent)
}
