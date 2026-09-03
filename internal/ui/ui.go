package ui

import (
	"io"
	"io/fs"
	"net/http"
	"path"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lspaya05/rmfakecloud-lite/internal/app/hub"
	"github.com/lspaya05/rmfakecloud-lite/internal/app/passcodestore"
	"github.com/lspaya05/rmfakecloud-lite/internal/common"
	"github.com/lspaya05/rmfakecloud-lite/internal/config"
	"github.com/lspaya05/rmfakecloud-lite/internal/messages"
	"github.com/lspaya05/rmfakecloud-lite/internal/screenshare"
	"github.com/lspaya05/rmfakecloud-lite/internal/storage"
	"github.com/lspaya05/rmfakecloud-lite/internal/storage/models"
	"github.com/lspaya05/rmfakecloud-lite/internal/ui/viewmodel"
	webui "github.com/lspaya05/rmfakecloud-lite/ui"
)

type backend interface {
	GetDocumentTree(uid string) (tree *viewmodel.DocumentTree, err error)
	Export(uid, doc, exporttype string, opt storage.ExportOption) (stream io.ReadCloser, err error)
	CreateDocument(uid, name, parent string, stream io.Reader) (doc *storage.Document, err error)
	CreateFolder(uid, name, parent string) (doc *storage.Document, err error)
	UpdateDocument(uid, docID, name, parent string) (err error)
	DeleteDocument(uid, docID string) (err error)
	Sync(uid string)
}
type codeGenerator interface {
	NewCode(string) (string, error)
}

type documentHandler interface {
	CreateDocument(uid, name, parent string, stream io.Reader) (doc *storage.Document, err error)
	CreateFolder(uid, name, parent string) (doc *storage.Document, err error)
	GetAllMetadata(uid string) (documents []*messages.RawMetadata, err error)
	ExportDocument(uid, id, format string, exportOption storage.ExportOption) (stream io.ReadCloser, err error)
	GetMetadata(uid, id string) (*messages.RawMetadata, error)
	UpdateMetadata(uid string, r *messages.RawMetadata) error
	RemoveDocument(uid, docid string) error
}

type blobHandler interface {
	GetCachedTree(uid string) (tree *models.HashTree, err error)
	CreateBlobDocument(uid, name, parent string, reader io.Reader) (doc *storage.Document, err error)
	UpdateBlobDocument(uid, docID, name, parent string) (err error)
	DeleteBlobDocument(uid, docID string) (err error)
	CreateBlobFolder(uid, name, parent string) (doc *storage.Document, err error)
	Export(uid, docid string) (io.ReadCloser, error)
	ExportRmDoc(uid, docid string) (io.ReadCloser, error)
}

type notificationHub interface {
	Deleted(uid, docID string) error
	Added(uid, docID string) error
	Updated(uid, docID string) error
	Sync(uid string) error
}

type mqttBridge interface {
	PublishSignaling(userID, clientID string, payload []byte)
	HasConnectedClient(userID string) bool
}

// ReactAppWrapper encapsulates an app
type ReactAppWrapper struct {
	fs            http.FileSystem
	prefix        string
	cfg           *config.Config
	userStorer    storage.UserStorer
	codeConnector codeGenerator
	h             *hub.Hub
	passcodeStore passcodestore.Store
	backends      map[common.SyncVersion]backend
	roomManager   *screenshare.RoomManager
	mqtt          mqttBridge
}

// hack for serving index.html on /
const indexReplacement = "/default"
const jsBuildFolder = "dist"

// New Create a React app
func New(cfg *config.Config,
	userStorer storage.UserStorer,
	codeConnector codeGenerator,
	h *hub.Hub,
	pcStore passcodestore.Store,
	docHandler documentHandler,
	blobHandler blobHandler,
	roomManager *screenshare.RoomManager,
	mqttBroker mqttBridge) *ReactAppWrapper {

	sub, err := fs.Sub(webui.Assets, jsBuildFolder)
	if err != nil {
		panic("not embedded?")
	}
	backend15 := &backend15{
		blobHandler: blobHandler,
		h:           h,
	}
	backend10 := &backend10{
		documentHandler: docHandler,
		hub:             h,
	}
	staticWrapper := ReactAppWrapper{
		fs:            common.NewLastModifiedFS(http.FS(sub), time.Now()),
		prefix:        "/assets",
		cfg:           cfg,
		userStorer:    userStorer,
		codeConnector: codeConnector,
		h:             h,
		passcodeStore: pcStore,
		backends: map[common.SyncVersion]backend{
			common.Sync10: backend10,
			common.Sync15: backend15,
		},
		roomManager: roomManager,
		mqtt:        mqttBroker,
	}
	return &staticWrapper
}

// Open opens a file from the fs (virtual)
func (w ReactAppWrapper) Open(filepath string) (http.File, error) {
	fullpath := filepath
	//index.html hack
	if filepath != indexReplacement {
		fullpath = path.Join(w.prefix, filepath)
	} else {
		fullpath = "/index.html"
	}
	f, err := w.fs.Open(fullpath)
	return f, err
}
func badReq(c *gin.Context, message string) {
	c.AbortWithStatusJSON(http.StatusBadRequest, viewmodel.NewErrorResponse(message))
}
