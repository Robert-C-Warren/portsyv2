package model

type ProjectDoc struct {
	ProjectID    string   `firestore:"-"            json:"projectId"`
	Name         string   `firestore:"name"         json:"name"`
	LastCommitID string   `firestore:"lastCommitId" json:"lastCommitId,omitempty"`
	LastCommitAt int64    `firestore:"lastCommitAt" json:"lastCommitAt,omitempty"`
	Last5        []string `firestore:"last5"        json:"last5,omitempty"`
}
