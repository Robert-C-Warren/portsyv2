package remote

import (
	"Portsy/backend/internal/core/model"
	"context"
	"fmt"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type MetaStore struct {
	client *firestore.Client
	projID string
}

type MetaStoreConfig struct {
	GCPProjectID      string // e.g. "portsy-prod"
	ServiceAccountKey string // path to service account json (or leave "" to use ADC)
}

// --- local, remote-only copies to avoid import cycles ---
type FileEntry struct {
	Path     string `firestore:"path" json:"path"`
	Hash     string `firestore:"hash" json:"hash"`
	Size     int64  `firestore:"size" json:"size"`
	Modified int64  `firestore:"modified" json:"modified"`
	R2Key    string `firestore:"r2Key" json:"r2Key"`
}

type ProjectState struct {
	ProjectName string      `firestore:"projectName" json:"projectName"`
	ProjectPath string      `firestore:"projectPath" json:"projectPath"`
	Files       []FileEntry `firestore:"files"       json:"files"`
	CreatedAt   int64       `firestore:"createdAt"   json:"createdAt"`
	Algo        string      `firestore:"algo"        json:"algo,omitempty"`
}

type CommitMeta struct {
	ID        string `firestore:"id"        json:"id"`
	Message   string `firestore:"message"   json:"message"`
	Timestamp int64  `firestore:"timestamp" json:"timestamp"`
	UserID    string `firestore:"userId"    json:"userId,omitempty"`
	ParentID  string `firestore:"parentId"  json:"parentId,omitempty"`
	Status    string `firestore:"status"    json:"status,omitempty"`
}

type ProjectDoc struct {
	ProjectID    string   `firestore:"-"            json:"projectId"`
	Name         string   `firestore:"name"         json:"name"`
	LastCommitID string   `firestore:"lastCommitId" json:"lastCommitId,omitempty"`
	LastCommitAt int64    `firestore:"lastCommitAt" json:"lastCommitAt,omitempty"`
	Last5        []string `firestore:"last5"        json:"last5,omitempty"`
}

func NewMetaStore(ctx context.Context, cfg MetaStoreConfig) (*MetaStore, error) {
	var (
		client *firestore.Client
		err    error
	)

	if cfg.ServiceAccountKey != "" {
		client, err = firestore.NewClient(ctx, cfg.GCPProjectID, option.WithCredentialsFile(cfg.ServiceAccountKey))
	} else {
		client, err = firestore.NewClient(ctx, cfg.GCPProjectID)
	}
	if err != nil {
		return nil, fmt.Errorf("firestore.NewClient: %w", err)
	}
	return &MetaStore{client: client, projID: cfg.GCPProjectID}, nil
}

func (m *MetaStore) Close() error {
	if m.client != nil {
		return m.client.Close()
	}
	return nil
}

// Collections layout:
// projects/{projectName}
//   - fields: Name, LastCommitID, LastCommitAt
//   - commits/{commitID} (doc)
//   - states/{commitID}  (doc)  // manifest snapshot for that commit
func (m *MetaStore) UpsertLatestState(ctx context.Context, projectName string, state ProjectState, commit CommitMeta) error {
	p := m.client.Collection("projects").Doc(projectName)

	// MergeAll REQUIRES a map, not a struct.
	if _, err := p.Set(ctx, map[string]interface{}{
		"Name":         projectName,
		"NameLower":    strings.ToLower(projectName),
		"LastCommitID": commit.ID,
		"LastCommitAt": commit.Timestamp,
	}, firestore.MergeAll); err != nil {
		return fmt.Errorf("upsert project header: %w", err)
	}

	// New commit doc — no merge needed.
	if _, err := p.Collection("commits").Doc(commit.ID).Set(ctx, commit); err != nil {
		return fmt.Errorf("set commit %s: %w", commit.ID, err)
	}

	// Snapshot for that commit.
	if _, err := p.Collection("states").Doc(commit.ID).Set(ctx, state); err != nil {
		return fmt.Errorf("set state %s: %w", commit.ID, err)
	}
	return nil
}

func (m *MetaStore) GetLatestState(ctx context.Context, projectName string) (*ProjectState, *CommitMeta, error) {
	p := m.client.Collection("projects").Doc(projectName)
	doc, err := p.Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("get project %q: %w", projectName, err)
	}

	var pd ProjectDoc
	if err := doc.DataTo(&pd); err != nil {
		return nil, nil, fmt.Errorf("decode project doc: %w", err)
	}
	if pd.LastCommitID == "" {
		return nil, nil, nil
	}

	cdoc, err := p.Collection("commits").Doc(pd.LastCommitID).Get(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("get commit %s: %w", pd.LastCommitID, err)
	}

	var cm CommitMeta
	if err := cdoc.DataTo(&cm); err != nil {
		return nil, nil, fmt.Errorf("decode commit %s: %w", pd.LastCommitID, err)
	}

	sdoc, err := p.Collection("states").Doc(pd.LastCommitID).Get(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("get state %s: %w", pd.LastCommitID, err)
	}

	var st ProjectState
	if err := sdoc.DataTo(&st); err != nil {
		return nil, nil, fmt.Errorf("decode state %s: %w", pd.LastCommitID, err)
	}
	return &st, &cm, nil
}

func (m *MetaStore) ListProjects(ctx context.Context) ([]model.ProjectDoc, error) {
	docs, err := m.client.Collection("projects").Documents(ctx).GetAll()
	if err != nil {
		return nil, err
	}
	out := make([]model.ProjectDoc, 0, len(docs))
	for _, d := range docs {
		var p model.ProjectDoc
		if err := d.DataTo(&p); err != nil {
			continue
		}
		p.ProjectID = d.Ref.ID
		out = append(out, p)
	}
	return out, nil
}

// BeginCommit writes a pending commit + its draft state.
// Only writes; no reads, so a batch is fine.
func (m *MetaStore) BeginCommit(ctx context.Context, projectName string, commit CommitMeta, state ProjectState) error {
	commit.Status = "pending"
	if commit.Timestamp == 0 {
		commit.Timestamp = time.Now().Unix()
	}

	p := m.client.Collection("projects").Doc(projectName)
	b := m.client.Batch()

	// Ensure the project doc exists (merge so we don't clobber fields)
	b.Set(p, map[string]any{
		"Name":      projectName,
		"NameLower": strings.ToLower(projectName),
	}, firestore.MergeAll)

	// Stash commit + state under subcollections
	b.Set(p.Collection("commits").Doc(commit.ID), commit)
	b.Set(p.Collection("states").Doc(commit.ID), state)

	_, err := b.Commit(ctx)
	if err != nil {
		return fmt.Errorf("begin commit %s: %w", commit.ID, err)
	}
	return nil
}

// FinalizeCommit verifies blobs exist (outside tx), then atomically:
// - writes the final commit + state (idempotent if already present)
// - advances project HEAD
// - updates Last5 as a list of commit IDs (max 5, oldest->newest)
func (m *MetaStore) FinalizeCommit(
	ctx context.Context,
	projectName string,
	commit CommitMeta,
	state ProjectState,
	verify func(context.Context, string) error, // verify(ctx, contentHashHex)
) error {
	// 1) Verify every file's blob exists in R2 BEFORE touching Firestore.
	for _, fe := range state.Files {
		if err := verify(ctx, fe.Hash); err != nil {
			return fmt.Errorf("verify blob %s: %w", fe.Hash, err)
		}
	}

	p := m.client.Collection("projects").Doc(projectName)
	commits := p.Collection("commits")
	states := p.Collection("states")

	// 2) Firestore transaction: all reads first, then writes (no read after write).
	return m.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		// READ the current project doc (ok before any writes)
		var proj ProjectDoc
		snap, err := tx.Get(p)
		if err != nil {
			if status.Code(err) == codes.NotFound {
				proj = ProjectDoc{Name: projectName}
			} else {
				return fmt.Errorf("tx get project: %w", err)
			}
		} else if err := snap.DataTo(&proj); err != nil {
			return fmt.Errorf("tx decode project: %w", err)
		}

		// Prepare the final commit
		commit.Status = "final"
		if commit.Timestamp == 0 {
			commit.Timestamp = time.Now().Unix()
		}

		// WRITE (no reads after this point)
		if err := tx.Set(commits.Doc(commit.ID), commit); err != nil {
			return fmt.Errorf("tx set commit: %w", err)
		}
		if err := tx.Set(states.Doc(commit.ID), state); err != nil {
			return fmt.Errorf("tx set state: %w", err)
		}

		// Advance HEAD + roll Last5 (IDs only)
		proj.Name = projectName
		proj.LastCommitID = commit.ID
		proj.LastCommitAt = commit.Timestamp

		// Append the new commit ID, clamp to last 5 (oldest -> newest)
		newLast := append(proj.Last5, commit.ID)
		if len(newLast) > 5 {
			newLast = newLast[len(newLast)-5:]
		}
		proj.Last5 = newLast

		// Upsert the project doc
		if err := tx.Set(p, proj); err != nil {
			return fmt.Errorf("tx set project: %w", err)
		}
		return nil
	})
}

func (m *MetaStore) GetCommitHistory(ctx context.Context, projectName string, limit int) ([]CommitMeta, error) {
	iter := m.client.Collection("projects").Doc(projectName).
		Collection("commits").OrderBy("Timestamp", firestore.Desc).Limit(limit).Documents(ctx)
	defer iter.Stop()

	var commits []CommitMeta
	for {
		d, err := iter.Next()
		if err != nil {
			if err == iterator.Done {
				break
			}
			return nil, fmt.Errorf("iterate commits: %w", err)
		}
		var cm CommitMeta
		if err := d.DataTo(&cm); err != nil {
			return nil, fmt.Errorf("decode commite: %w", err)
		}
		commits = append(commits, cm)
	}
	return commits, nil
}

// Fetch manifest + commit metadata for a specific commit ID.
func (m *MetaStore) GetStateByCommit(ctx context.Context, projectName, commitID string) (*ProjectState, *CommitMeta, error) {
	p := m.client.Collection("projects").Doc(projectName)

	cdoc, err := p.Collection("commits").Doc(commitID).Get(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("get commit %s: %w", commitID, err)
	}
	var cm CommitMeta
	if err := cdoc.DataTo(&cm); err != nil {
		return nil, nil, fmt.Errorf("decode commit %s: %w", commitID, err)
	}

	sdoc, err := p.Collection("states").Doc(commitID).Get(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("get state %s: %w", commitID, err)
	}
	var st ProjectState
	if err := sdoc.DataTo(&st); err != nil {
		return nil, nil, fmt.Errorf("decode state %s: %w", commitID, err)
	}
	return &st, &cm, nil
}
