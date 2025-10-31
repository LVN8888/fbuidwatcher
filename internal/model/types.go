package model

type WatchInfo struct {
	UID         string `json:"uid"`
	Note        string `json:"note,omitempty"`
	IntervalSec int    `json:"interval"`
	LastStatus  *bool  `json:"last_status,omitempty"`
	AddedAtUnix int64  `json:"added_at_unix"`
}

type OwnerData struct {
	DefaultIntervalSec int                  `json:"default_interval"`
	Items              map[string]WatchInfo `json:"items"`
}

type DataStore map[string]OwnerData
