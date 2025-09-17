package uiapi

import (
	remote "Portsy/backend/remote"
	"context"
	"os"
)

type API struct {
	ctx       context.Context
	MetaStore *remote.MetaStore
}

func (a *API) SetContext(ctx context.Context) { a.ctx = ctx }

// Call once on startup
func (a *API) InitMetaStore(projectId, serviceAccountPath string) error {
	ms, err := remote.NewMetaStore(a.ctx, remote.MetaStoreConfig{
		GCPProjectID:      projectId,
		ServiceAccountKey: serviceAccountPath,
	})
	if err != nil {
		return err
	}
	a.MetaStore = ms
	return nil
}

// Shape returned to the frontend pull panel
type RemoteProject struct {
	ProjectID    string `json:"projectId"`
	Name         string `json:"name"`
	LastCommitID string `json:"lastCommitId"`
	LastCommitAt int64  `json:"lastCommitAt"`
}

// SHows up as window.go.uiapi.API.ListRemoteProjects()
func (a *API) ListRemoteProjects() (map[string]any, error) {
	if a.MetaStore == nil {
		_ = a.InitMetaStore(os.Getenv("FIREBASE_PROJECT_ID"), os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"))
	}
	if a.MetaStore == nil {
		return map[string]any{"ok": false, "error": "metastore not initialized"}, nil
	}

	projs, err := a.MetaStore.ListProjects(a.ctx)
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error()}, nil
	}

	items := make([]RemoteProject, 0, len(projs))
	for _, p := range projs {
		items = append(items, RemoteProject{
			ProjectID:    p.ProjectID,
			Name:         p.Name,
			LastCommitID: p.LastCommitID,
			LastCommitAt: p.LastCommitAt,
		})
	}
	return map[string]any{"ok": true, "count": len(items), "items": items}, nil
}
