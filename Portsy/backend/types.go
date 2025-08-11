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
}

// Commit metadata stored in Firestore
type CommitMeta struct {
	ID        string `json:"id"`
	Message   string `json:"message"`
	Timestamp int64  `json:"timestamp"`
	UserID    string `json:"userId,omitempty"`
}

// Firestore document we keep as "latest" pointer
type ProjectDoc struct {
	Name         string `json:"name"`
	LastCommitID string `json:"lastCommitId,omitempty"`
	LastCommitAt int64  `json:"lastCommitAt,omitempty"`
}
