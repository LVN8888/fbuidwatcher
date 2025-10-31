package model

type WatchInfo struct {
	UID         string `json:"uid"`
	Note        string `json:"note,omitempty"`
	IntervalSec int    `json:"interval"`
	LastStatus  *bool  `json:"last_status,omitempty"`
	AddedAtUnix int64  `json:"added_at_unix"`
}

// DataStore: ownerID -> (uid -> WatchInfo)
type DataStore map[string]map[string]WatchInfo
