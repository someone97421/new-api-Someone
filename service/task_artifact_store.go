package service

import (
	"context"
	"errors"
	"io"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

// StoredArtifactRef describes a persisted artifact object. No reference is
// produced until a concrete storage backend is implemented.
type StoredArtifactRef struct {
	Backend   string
	Bucket    string
	ObjectKey string
	MimeType  string
	Size      int64
}

// TaskArtifactStore is the persistence boundary for generated artifact bytes.
// types.TaskArtifact is re-exported by relay/channel as channel.TaskArtifact.
type TaskArtifactStore interface {
	Enabled() bool
	Resolve(task *model.Task, artifactKey string) (*StoredArtifactRef, error)
	Persist(ctx context.Context, task *model.Task, artifact types.TaskArtifact, content io.Reader) (*StoredArtifactRef, error)
	Serve(c *gin.Context, task *model.Task, ref *StoredArtifactRef) error
}

// ArtifactCleaner is implemented by storage backends that support expired artifact cleanup.
type ArtifactCleaner interface {
	CleanExpired(ctx context.Context) (int, error)
}

var ErrTaskArtifactStoreDisabled = errors.New("task artifact store is disabled")

type disabledArtifactStore struct{}

func (disabledArtifactStore) Enabled() bool {
	return false
}

func (disabledArtifactStore) Resolve(*model.Task, string) (*StoredArtifactRef, error) {
	return nil, nil
}

func (disabledArtifactStore) Persist(context.Context, *model.Task, types.TaskArtifact, io.Reader) (*StoredArtifactRef, error) {
	return nil, ErrTaskArtifactStoreDisabled
}

func (disabledArtifactStore) Serve(*gin.Context, *model.Task, *StoredArtifactRef) error {
	return ErrTaskArtifactStoreDisabled
}

var taskArtifactStore TaskArtifactStore = &disabledArtifactStore{}

// InitTaskArtifactStore initializes the process-wide artifact store from configuration.
func InitTaskArtifactStore(cfg system_setting.TaskArtifactStoreConfig) {
	if cfg.Mode == system_setting.TaskArtifactStoreModeLocal {
		taskArtifactStore = NewLocalArtifactStore(cfg.LocalDir, cfg.RetentionHours)
	} else {
		taskArtifactStore = &disabledArtifactStore{}
	}
}

// SetTaskArtifactStore explicitly sets the process-wide store instance (e.g. for testing).
func SetTaskArtifactStore(store TaskArtifactStore) {
	if store == nil {
		taskArtifactStore = &disabledArtifactStore{}
		return
	}
	taskArtifactStore = store
}

func init() {
	cfg := system_setting.LoadTaskArtifactStoreConfig()
	InitTaskArtifactStore(cfg)
}

// GetTaskArtifactStore returns the process-wide artifact storage backend.
func GetTaskArtifactStore() TaskArtifactStore {
	return taskArtifactStore
}
