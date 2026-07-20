package app

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ddvk/rmfakecloud/internal/app/hub"
	"github.com/ddvk/rmfakecloud/internal/common"
	"github.com/ddvk/rmfakecloud/internal/config"
	"github.com/ddvk/rmfakecloud/internal/integrations"
	"github.com/ddvk/rmfakecloud/internal/messages"
	mqttmod "github.com/ddvk/rmfakecloud/internal/mqtt"
	"github.com/ddvk/rmfakecloud/internal/storage/fs"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v4"
	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
)

const (
	internalErrorMessage = "Internal Error"
	handlerLog           = "[handler] "
	// a way to invalidate the user token
	tokenVersion   = 10
	maxRequestSize = 7000000000
)

func (app *App) getDeviceClaims(c *gin.Context) (*DeviceClaims, error) {
	token, err := common.GetToken(c)
	if err != nil {
		return nil, err
	}
	claims := &DeviceClaims{}
	err = common.ClaimsFromToken(claims, token, app.cfg.JWTSecretKey)
	if err != nil {
		return nil, err
	}
	if claims.UserID == "" {
		return nil, fmt.Errorf("wrong token, missing userid")
	}
	return claims, nil
}

func (app *App) getUserClaims(c *gin.Context) (*UserClaims, error) {
	token, err := common.GetToken(c)
	// log.Debug(handlerLog, "Token: ", token)
	if err != nil {
		return nil, err
	}
	claims := &UserClaims{}
	err = common.ClaimsFromToken(claims, token, app.cfg.JWTSecretKey)
	if err != nil {
		return nil, err
	}
	if claims.Profile.UserID == "" {
		return nil, fmt.Errorf("wrong token, missing userid")
	}
	if claims.Version != tokenVersion {
		return nil, fmt.Errorf("wrong token version, something has changed")
	}
	return claims, nil
}

func (app *App) newDevice(c *gin.Context) {
	var tokenRequest messages.DeviceTokenRequest
	if err := c.ShouldBindJSON(&tokenRequest); err != nil {
		badReq(c, err.Error())
		return
	}

	code := strings.ToLower(tokenRequest.Code)
	log.Info("Got code ", code)

	uid, err := app.codeConnector.ConsumeCode(code)
	if err != nil {
		log.Warn(err)
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	log.Info("Request: ", tokenRequest, "Token for:", uid)

	// generate the JWT token
	claims := &DeviceClaims{
		DeviceDesc: tokenRequest.DeviceDesc,
		DeviceID:   tokenRequest.DeviceID,
		UserID:     uid,
		StandardClaims: jwt.StandardClaims{
			Audience: APIUsage,
		},
	}

	tokenString, err := common.SignClaims(claims, app.cfg.JWTSecretKey)
	if err != nil {
		log.Warn(err)
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	c.String(http.StatusOK, tokenString)
}

func (app *App) deleteDevice(c *gin.Context) {
	deviceToken, err := app.getDeviceClaims(c)
	if err != nil {
		log.Error(err)
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	log.Info("Logging out: ", deviceToken.UserID)
	c.Status(http.StatusNoContent)
}

func (app *App) newUserToken(c *gin.Context) {
	deviceToken, err := app.getDeviceClaims(c)
	if err != nil {
		log.Error(err)
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	uid := strings.TrimPrefix(deviceToken.UserID, "auth0|")

	user, err := app.userStorer.GetUser(uid)
	if err != nil {
		log.Error(err)
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	if user == nil {
		log.Warn("User not found: ", uid)
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	scopes := []string{"intgr", "screenshare", "docedit"}

	if user.Sync15 {
		log.Info("Using sync 1.5")
		scopes = append(scopes, syncNew)
	} else {
		scopes = append(scopes, syncDefault)
	}

	if len(user.AdditionalScopes) > 0 {
		scopes = append(scopes, user.AdditionalScopes...)
	}

	scopesStr := strings.Join(scopes, " ")
	log.Info("setting scopes: ", scopesStr)

	jti := make([]byte, 3)
	_, err = rand.Read(jti)
	if err != nil {
		badReq(c, err.Error())
		return
	}
	jti = append([]byte{'r', 'M', '-'}, jti...)
	jti = append(jti, '/', 'E')

	now := time.Now()
	expirationTime := now.Add(3 * time.Hour)
	claims := &UserClaims{
		Profile: Auth0profile{
			UserID:        deviceToken.UserID,
			IsSocial:      false,
			Connection:    "Username-Password-Authentication",
			Name:          user.Email,
			Nickname:      user.Nickname,
			GivenName:     user.Name,
			Email:         fmt.Sprintf("%s (via %s)", user.Email, app.cfg.StorageURL),
			EmailVerified: true,
			CreatedAt:     user.CreatedAt,
			UpdatedAt:     user.UpdatedAt,
		},
		DeviceDesc: deviceToken.DeviceDesc,
		DeviceID:   deviceToken.DeviceID,
		Scopes:     scopesStr,
		Level:      "connect",
		Tectonic:   "eu",
		StandardClaims: jwt.StandardClaims{
			ExpiresAt: expirationTime.Unix(),
			NotBefore: now.Unix(),
			IssuedAt:  now.Unix(),
			Subject:   deviceToken.UserID,
			Issuer:    "rM WebApp",
			Id:        base64.StdEncoding.EncodeToString(jti),
		},
		Version: tokenVersion,
	}

	tokenString, err := common.SignClaims(claims, app.cfg.JWTSecretKey)
	if err != nil {
		badReq(c, err.Error())
		return
	}
	c.String(http.StatusOK, tokenString)
}

func userID(c *gin.Context) string {
	//TODO: suppress the warning
	//codeql[go/path-injection]
	return c.GetString(userIDKey)
}

func (app *App) listDocuments(c *gin.Context) {

	uid := userID(c)
	withBlob, _ := strconv.ParseBool(c.Query("withBlob"))
	docID := common.QueryS("doc", c)
	log.Debug(handlerLog, "params: withBlob: ", withBlob, ", DocId: ", docID)
	result := []*messages.RawMetadata{}

	var err error
	if docID != "" {
		//load single document
		var doc *messages.RawMetadata
		doc, err = app.metaStorer.GetMetadata(uid, docID)
		if err == nil {
			result = append(result, doc)
		}
	} else {
		//load all
		result, err = app.metaStorer.GetAllMetadata(uid)
	}

	if err != nil {
		log.Error(err)
		internalError(c, "cant get metadata")
		return
	}

	for _, response := range result {
		if withBlob {
			storageURL, exp, err := app.docStorer.GetStorageURL(uid, response.ID)
			if err != nil {
				response.Success = false
				log.Warn("Cant get storage url for : ", response.ID)
				continue
			}
			response.BlobURLGet = storageURL
			response.BlobURLGetExpires = exp.UTC().Format(time.RFC3339Nano)
		} else {
			response.BlobURLGetExpires = time.Time{}.Format(time.RFC3339Nano)
		}
		response.Success = true
	}

	c.JSON(http.StatusOK, result)
}
func (app *App) deleteDocument(c *gin.Context) {
	uid := userID(c)
	deviceID := c.GetString(deviceIDKey)

	var req []messages.IDRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		log.Warn(err)
		badReq(c, err.Error())
		return
	}

	result := []messages.StatusResponse{}
	for _, r := range req {
		doc, err := app.metaStorer.GetMetadata(uid, r.ID)
		ok := false
		if err == nil {
			err := app.docStorer.RemoveDocument(uid, r.ID)
			if err != nil {
				log.Error(err)
			} else {
				ok = true

				ntf := hub.DocumentNotification{
					ID:      doc.ID,
					Type:    doc.Type,
					Version: doc.Version,
					Parent:  doc.Parent,
					Name:    doc.VissibleName,
				}
				app.hub.Notify(uid, deviceID, ntf, messages.DocDeletedEvent)
			}
		}
		result = append(result, messages.StatusResponse{ID: r.ID, Success: ok})
	}

	c.JSON(http.StatusOK, result)
}
func (app *App) updateStatus(c *gin.Context) {
	uid := userID(c)
	deviceID := c.GetString(deviceIDKey)
	var req []messages.RawMetadata

	if err := c.ShouldBindJSON(&req); err != nil {
		log.Error(err)
		badReq(c, err.Error())
		return
	}
	result := []messages.StatusResponse{}
	for _, doc := range req {
		log.Info("Id: ", doc.ID, " Name: ", doc.VissibleName)

		message := ""

		ok := false
		err := app.metaStorer.UpdateMetadata(uid, &doc)
		if err != nil {
			message = internalErrorMessage
			log.Error(err)
		} else {
			ok = true

			ntf := hub.DocumentNotification{
				ID:      doc.ID,
				Type:    doc.Type,
				Version: doc.Version,
				Parent:  doc.Parent,
				Name:    doc.VissibleName,
			}

			app.hub.Notify(uid, deviceID, ntf, messages.DocAddedEvent)
		}
		result = append(result, messages.StatusResponse{ID: doc.ID, Success: ok, Message: message, Version: doc.Version})
	}

	c.JSON(http.StatusOK, result)
}

func (app *App) locateService(c *gin.Context) {
	// old api < 3 something
	svc := c.Param("service")
	log.Infof("Requested: %s\n", svc)
	host := config.DefaultHost
	if svc == "blob-storage" {
		host = "https://" + config.DefaultHost
	}
	response := messages.HostResponse{Host: host, Status: "OK"}
	c.JSON(http.StatusOK, response)
}
func (app *App) syncComplete(c *gin.Context) {
	log.Info("Sync complete")
	uid := userID(c)
	deviceID := c.GetString(deviceIDKey)

	var res messages.SyncCompleted
	res.ID = app.hub.NotifySync(uid, deviceID)
	c.JSON(http.StatusOK, res)
}

func (app *App) syncCompleteV2(c *gin.Context) {
	log.Info("Sync completeV2")
	uid := userID(c)
	deviceID := c.GetString(deviceIDKey)

	var req messages.SyncCompletedRequestV2
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Error(err)
		badReq(c, err.Error())
		return
	}
	log.Info("got sync completed, gen: ", req.Generation)

	notificationID := app.hub.NotifySync(uid, deviceID)

	res := messages.SyncCompleted{
		ID: notificationID,
	}
	c.JSON(http.StatusOK, res)
}
func formatExpires(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func (app *App) blobStorageDownload(c *gin.Context) {
	uid := userID(c)
	var req messages.BlobStorageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Error(err)
		badReq(c, err.Error())
		return
	}
	if req.RelativePath == "" {
		badReq(c, "no rel")
		return
	}

	url, exp, err := app.blobStorer.GetBlobURL(uid, req.RelativePath, false)
	if err != nil {
		log.Error(err)
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	response := messages.BlobStorageResponse{
		Method:       http.MethodGet,
		RelativePath: req.RelativePath,
		URL:          url,
		Expires:      formatExpires(exp),
	}
	c.JSON(http.StatusOK, response)
}

func (app *App) blobStorageUpload(c *gin.Context) {
	var req messages.BlobStorageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Error(err)
		badReq(c, err.Error())
		return
	}
	if req.RelativePath == "" {
		badReq(c, "no rel")
		return
	}
	if req.Initial {
		log.Info("--- Initial Sync ---")
	}
	uid := userID(c)
	url, exp, err := app.blobStorer.GetBlobURL(uid, req.RelativePath, true)
	if err != nil {
		log.Error(err)
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	response := messages.BlobStorageResponse{
		Method:         http.MethodPut,
		RelativePath:   req.RelativePath,
		URL:            url,
		Expires:        formatExpires(exp),
		MaxRequestSize: maxRequestSize,
	}

	c.JSON(http.StatusOK, response)
}

func (app *App) syncUpdateRootV3(c *gin.Context) {
	var rootv3 messages.SyncRootV3Request
	err := c.BindJSON(&rootv3)
	if err != nil {
		log.Error(err)
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	uid := userID(c)
	newgeneration, err := app.blobStorer.StoreBlob(uid, RootHash, bytes.NewBufferString(rootv3.Hash), rootv3.Generation)
	if err != nil {
		log.Error(err)
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	if rootv3.Broadcast {
		deviceID := c.GetString(deviceIDKey)

		log.Info("got sync completed, gen: ", newgeneration)

		app.hub.NotifySync(uid, deviceID)
	}

	c.JSON(http.StatusOK, messages.SyncRootV3Response{
		Generation: newgeneration,
		Hash:       rootv3.Hash,
	})
}

const SchemaVersion = 3

const RmTokenTtlHeader = "Rm-Token-Ttl-Hint"
const RmFileHeader = "rm-filename"

const RootHash = "root"

// crcJSON calculates and ands the crc32c header
// TODO: fix it with a custom render or something
func crcJSON(c *gin.Context, status int, msg any) {
	b, err := json.Marshal(msg)
	if err != nil {
		panic(err)
	}

	crc, err := common.CRC32CFromReader(bytes.NewBuffer(b))
	if err != nil {
		panic(err)
	}
	common.AddHashHeader(c, "crc32c="+crc)
	c.Data(status, "application/json", b)
}

func (app *App) syncGetRootV3(c *gin.Context) {
	uid := userID(c)
	reader, generation, _, _, err := app.blobStorer.LoadBlob(uid, RootHash)
	if err == fs.ErrorNotFound {
		log.Warn("No root file found, assuming this is a new account")
		c.JSON(http.StatusNotFound, gin.H{"message": "root not found"})
		return
	}

	if err != nil {
		log.Error(err)
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	roothash, err := io.ReadAll(reader)
	if err != nil {
		log.Error(err)
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	c.JSON(http.StatusOK, messages.SyncRootV3Response{
		Generation: generation,
		Hash:       string(roothash),
	})
}

func (app *App) syncGetRootV4(c *gin.Context) {
	uid := userID(c)
	reader, generation, _, _, err := app.blobStorer.LoadBlob(uid, RootHash)
	if err == fs.ErrorNotFound {
		log.Warn("No root file found, assuming this is a new account")
		crcJSON(c, http.StatusOK, messages.SyncRootV4Response{
			SchemaVersion: SchemaVersion,
		})
		return
	}

	if err != nil {
		log.Error(err)
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	roothash, err := io.ReadAll(reader)
	if err != nil {
		log.Error(err)
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	crcJSON(c, http.StatusOK, messages.SyncRootV4Response{
		Generation:    generation,
		Hash:          string(roothash),
		SchemaVersion: SchemaVersion,
	})
}

func (app *App) checkFilesPresence(c *gin.Context) {
	uid := userID(c)
	var req messages.CheckFiles
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Error(err)
		badReq(c, err.Error())
		return
	}

	mfs := messages.MissingFiles{}

	for _, fileid := range req.Files {
		_, _, err := app.blobStorer.GetBlobURL(uid, fileid, false)
		if err != nil {
			mfs.MissingFiles = append(mfs.MissingFiles, fileid)
		}
	}

	c.JSON(http.StatusOK, mfs)
}

func (app *App) checkMissingBlob(c *gin.Context) {
	mhs := messages.MissingHashes{}

	// TODO

	c.JSON(http.StatusOK, mhs)
}

func (app *App) blobStorageRead(c *gin.Context) {
	uid := userID(c)
	blobID := common.ParamS(fileKey, c)

	reader, _, size, hash, err := app.blobStorer.LoadBlob(uid, blobID)
	if err == fs.ErrorNotFound {
		log.Warn(err)
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	if err != nil {
		log.Error(err)
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	defer reader.Close()
	common.AddHashHeader(c, hash)

	c.DataFromReader(http.StatusOK, size, "application/octet-stream", reader, nil)
}

func (app *App) blobStorageWrite(c *gin.Context) {
	uid := userID(c)
	blobID := common.ParamS(fileKey, c)

	fileName := c.GetHeader(RmFileHeader)
	hash := c.GetHeader(common.GCPHashHeader)
	log.Debugf("TODO: check/save etc. write file '%s', hash '%s'", fileName, hash)

	_, err := app.blobStorer.StoreBlob(uid, blobID, c.Request.Body, 0)
	if err != nil {
		log.Error(err)
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	c.Status(http.StatusOK)
}

func (app *App) integrationsCalendarEvents(c *gin.Context) {
	uid := userID(c)
	integrationID := common.ParamS(integrationKey, c)

	provider, err := integrations.GetCalendarIntegrationProvider(app.userStorer, uid, integrationID)
	if err != nil {
		log.Error(fmt.Errorf("can't get calendar integration, %v", err))
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	now := time.Now().UTC()
	response, err := provider.ListEvents(now, now.Add(24*time.Hour))
	if err != nil {
		log.Error(err)
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	c.JSON(http.StatusOK, response)
}

func (app *App) integrations(c *gin.Context) {
	uid := userID(c)

	response, err := integrations.List(app.userStorer, uid)

	if err != nil {
		log.Error(err)
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	if !supportsCalendarIntegration(c.GetHeader("User-Agent")) {
		filtered := &messages.IntegrationsResponse{}
		for _, intg := range response.Integrations {
			if intg.ProviderType != "Calendar" {
				filtered.Integrations = append(filtered.Integrations, intg)
			}
		}
		response = filtered
	}

	c.JSON(http.StatusOK, response)
}

var xochitlVersionRe = regexp.MustCompile(`xochitl/(\d+)\.(\d+)`)

func supportsCalendarIntegration(ua string) bool {
	m := xochitlVersionRe.FindStringSubmatch(ua)
	if m == nil {
		return true
	}
	major, _ := strconv.Atoi(m[1])
	minor, _ := strconv.Atoi(m[2])
	return major > 3 || (major == 3 && minor >= 27)
}

func (app *App) uploadRequest(c *gin.Context) {
	uid := userID(c)
	var req []messages.UploadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Errorf("could not bind %v", err)
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	response := []messages.UploadResponse{}

	for _, r := range req {
		documentID := r.ID
		if documentID == "" {
			badReq(c, "no id")
		}
		url, exp, err := app.docStorer.GetStorageURL(uid, documentID)
		if err != nil {
			log.Error(err)
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		log.Debugln("StorageUrl: ", url)
		dr := messages.UploadResponse{
			BlobURLPut:        url,
			BlobURLPutExpires: exp.UTC().Format(time.RFC3339Nano),
			ID:                documentID,
			Success:           true,
			Version:           r.Version,
		}
		response = append(response, dr)
	}

	c.JSON(http.StatusOK, response)
}

func (app *App) connectWebSocket(c *gin.Context) {
	uid := userID(c)
	deviceID := c.GetString(deviceIDKey)

	log.Info("connecting websocket from: ", uid)

	var upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
	connection, err := upgrader.Upgrade(c.Writer, c.Request, nil)

	if err != nil {
		log.Warn("can't upgrade websocket to ws ", err)
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	go app.hub.ConnectWs(uid, deviceID, connection)
}

func (app *App) screenshareCreateRoom(c *gin.Context) {
	uid := userID(c)
	deviceID := c.GetString(deviceIDKey)

	room := app.roomManager.CreateRoom(uid, deviceID)

	c.JSON(http.StatusCreated, gin.H{
		"roomId":     room.RoomID,
		"createdAt":  room.CreatedAt.Format(time.RFC3339Nano),
		"iceServers": app.cfg.ICEServers,
	})
}

func (app *App) screenshareDeleteRoom(c *gin.Context) {
	app.roomManager.DeleteRoom(c.Param("roomId"))
	c.Status(http.StatusNoContent)
}

func (app *App) screenshareJoinActive(c *gin.Context) {
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

func (app *App) screenshareKeepalive(c *gin.Context) {
	app.roomManager.Keepalive(c.Param("roomId"))
	c.Status(http.StatusOK)
}

func (app *App) screenshareGetRoom(c *gin.Context) {
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

func (app *App) screenshareBroadcast(c *gin.Context) {
	roomID := c.Param("roomId")
	deviceID := c.GetString(deviceIDKey)

	if !app.roomManager.RoomExists(roomID) {
		c.JSON(http.StatusNotFound, gin.H{"error": "room not found"})
		return
	}

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		badReq(c, "invalid body")
		return
	}

	log.Debugf("Screenshare broadcast: room=%s from=%s payload=%s", roomID, deviceID, string(body))

	app.roomManager.AddBroadcast(roomID, deviceID, body)
	c.Status(http.StatusOK)
}

func (app *App) screenshareDirect(c *gin.Context) {
	roomID := c.Param("roomId")
	deviceID := c.GetString(deviceIDKey)

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

	log.Debugf("Screenshare direct: room=%s from=%s to=%s", roomID, deviceID, msg.TargetClientID)

	app.roomManager.AddDirect(roomID, deviceID, msg.TargetClientID, msg.Payload)
	c.Status(http.StatusOK)
}

func (app *App) handleMQTTWebSocket(c *gin.Context) {
	upgrader := websocket.Upgrader{
		Subprotocols: []string{"mqtt"},
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Warnf("MQTT WebSocket upgrade failed: %v", err)
		return
	}

	err = app.mqttBroker.EstablishConnection("ws", mqttmod.NewWsConn(conn))
	if err != nil {
		log.Warnf("MQTT WebSocket connection failed: %v", err)
	}
}

// syncReports reports sync errors back
func (app *App) syncReports(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)

	if err != nil {
		log.Warn("cant parse sync report, ignored")
		c.Status(http.StatusOK)
		return
	}
	log.Infof("got sync report: %s", string(body))
	c.Status(http.StatusOK)
}

func (app *App) analyticsReport(c *gin.Context) {
	// _, err := io.ReadAll(c.Request.Body)
	// if err != nil {
	// 	log.Warn("could not read report data")
	// }

	c.JSON(http.StatusCreated, gin.H{"message": "Success"})
}

func (app *App) nullReport(c *gin.Context) {
	// _, err := io.ReadAll(c.Request.Body)
	// if err != nil {
	// 	log.Warn("could not read report data")
	// }
	c.Status(http.StatusOK)
}
