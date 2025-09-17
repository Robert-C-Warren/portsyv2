package backend

// A single file tracked in a manifest/version.
type FileEntry struct {
	Path     string `json:"path"` // Relative to project root
	Hash     string `json:"hash"` // sha256
	Size     int64  `json:"size"`
	Modified int64  `json:"modified"` // Unix Seconds
	R2Key    string `json:"r2Key"`    // Where it lives in R2 (optional if deduping by hash)
}

// The state of a project at a point in time (used for diffing).
type ProjectState struct {
	ProjectName string      `json:"projectName"`
	ProjectPath string      `json:"projectPath"` // local (for context only)
	Files       []FileEntry `json:"files"`
	CreatedAt   int64       `json:"createdAt"`
	Algo        string      `json:"algo,omitempty"` // sha256 or blake3
}

// Commit metadata stored in Firestore
type CommitMeta struct {
	ID        string `json:"id"`
	Message   string `json:"message"`
	Timestamp int64  `json:"timestamp"`
	UserID    string `json:"userId,omitempty"`
	ParentID  string `json:"parentId,omitempty"`
	Status    string `json:"status,omitempty"` // "pending" | "final"
}

// Firestore document we keep as "latest" pointer
type ProjectDoc struct {
	ProjectID    string   `firestore:"-"              json:"projectId"`
	Name         string   `firestore:"name"           json:"name"`
	LastCommitID string   `firestore:"lastCommitId"   json:"lastCommitId,omitempty"`
	LastCommitAt int64    `firestore:"lastCommitAt"   json:"lastCommitAt,omitempty"`
	Last5        []string `firestore:"last5"          json:"last5,omitempty"`
}

type Diff struct {
	Added   []FileEntry `json:"added"`
	Changed []FileEntry `json:"changed"`
	Removed []FileEntry `json:"removed"`
}

type DiffSummary struct {
	ProjectID string   `json:"projectId"`
	Added     []string `json:"added"`
	Modified  []string `json:"modified"`
	Deleted   []string `json:"deleted"`
}

type ProjectSummary struct {
	Name            string `json:"name"`
	HasLocalChanges bool   `json:"hasLocalChanges"`
	CreatedLocally  bool   `json:"createdLocally"`
	Stats           struct {
		Added   int `json:"added" firestore:"-"`
		Changed int `json:"changed" firestore:"-"`
		Removed int `json:"removed" firestore:"-"`
	} `json:"stats"`
	LastCommitID string `json:"lastCommitId,omitempty"`
}

type PullStats struct {
	ToDownload int `json:"toDownload"`
	Downloaded int `json:"downloaded"`
	Verified   int `json:"verified"`
	Deleted    int `json:"deleted"`
	Skipped    int `json:"skipped"`
}

type PullStatus struct {
	LocalNewer bool   `json:"localNewer"`
	RemoteHead string `json:"remoteHead,omitempty"`
	LocalHead  string `json:"localhead,omitempty"`
}

type Config struct {
	UserID           string
	FirestoreProject string
	R2Account        string
	R2Bucket         string
	R2AccessKey      string
	R2SecretKey      string
}
