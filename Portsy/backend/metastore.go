package backend

import (
	"context"

	// use your real module path:
	// "github.com/Robert-C-Warren/portsyv2/Portsy/backend/remote"
	"Portsy/backend/remote"
)

// What app.go and others depend on.
type MetaStore interface {
	UpsertLatestState(ctx context.Context, project string, state ProjectState, commit CommitMeta) error
	GetLatestState(ctx context.Context, project string) (*ProjectState, error)
	GetStateByCommit(ctx context.Context, project, commitID string) (*ProjectState, error)
}

type MetaStoreConfig struct {
	ProjectID       string
	CredentialsPath string
	EmulatorHost    string // optional
}

// Keep call-site simple: just pass cfg (no context parameter needed here).
func NewMetaStore(cfg MetaStoreConfig) (MetaStore, error) {
	return remote.NewFirebaseStore(remote.Config{
		ProjectID:       cfg.ProjectID,
		CredentialsPath: cfg.CredentialsPath,
		EmulatorHost:    cfg.EmulatorHost,
	})
}
