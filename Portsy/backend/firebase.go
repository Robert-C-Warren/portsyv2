package backend

import (
	"context"
	"fmt"

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

func NewMetaStore(ctx context.Context, cfg MetaStoreConfig) (*MetaStore, error) {
	var client *firestore.Client
	var err error

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
		"LastCommitID": commit.ID,
		"LastCommitAt": commit.Timestamp,
	}, firestore.MergeAll); err != nil {
		return err
	}

	// New commit doc — no merge needed.
	if _, err := p.Collection("commits").Doc(commit.ID).Set(ctx, commit); err != nil {
		return err
	}

	// Snapshot for that commit.
	if _, err := p.Collection("states").Doc(commit.ID).Set(ctx, state); err != nil {
		return err
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
		return nil, nil, err
	}
	var pd ProjectDoc
	if err := doc.DataTo(&pd); err != nil {
		return nil, nil, err
	}
	if pd.LastCommitID == "" {
		return nil, nil, nil
	}

	cdoc, err := p.Collection("commits").Doc(pd.LastCommitID).Get(ctx)
	if err != nil {
		return nil, nil, err
	}
	var cm CommitMeta
	if err := cdoc.DataTo(&cm); err != nil {
		return nil, nil, err
	}

	sdoc, err := p.Collection("states").Doc(pd.LastCommitID).Get(ctx)
	if err != nil {
		return nil, nil, err
	}
	var st ProjectState
	if err := sdoc.DataTo(&st); err != nil {
		return nil, nil, err
	}
	return &st, &cm, nil
}

func (m *MetaStore) ListProjects(ctx context.Context) ([]ProjectDoc, error) {
	iter := m.client.Collection("projects").Documents(ctx)
	var out []ProjectDoc
	for {
		d, err := iter.Next()
		if err != nil {
			if err == iterator.Done {
				break
			}
			return nil, err
		}
		var pd ProjectDoc
		if err := d.DataTo(&pd); err != nil {
			return nil, err
		}
		out = append(out, pd)
	}
	return out, nil
}

func (m *MetaStore) GetCommitHistory(ctx context.Context, projectName string, limit int) ([]CommitMeta, error) {
	iter := m.client.Collection("projects").Doc(projectName).
		Collection("commits").OrderBy("Timestamp", firestore.Desc).Limit(limit).Documents(ctx)

	var commits []CommitMeta
	for {
		d, err := iter.Next()
		if err != nil {
			if err == iterator.Done {
				break
			}
			return nil, err
		}
		var cm CommitMeta
		if err := d.DataTo(&cm); err != nil {
			return nil, err
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
		return nil, nil, err
	}
	var cm CommitMeta
	if err := cdoc.DataTo(&cm); err != nil {
		return nil, nil, err
	}

	sdoc, err := p.Collection("states").Doc(commitID).Get(ctx)
	if err != nil {
		return nil, nil, err
	}
	var st ProjectState
	if err := sdoc.DataTo(&st); err != nil {
		return nil, nil, err
	}
	return &st, &cm, nil
}
