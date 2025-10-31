package storage

import (
	"encoding/json"
	"fbuidwatcher/internal/model"
	"os"
	"sync"
)

type FileStore struct {
	path string
	mu   sync.Mutex
}

func NewFileStore(path string) *FileStore { return &FileStore{path: path} }

func (s *FileStore) Load() (model.DataStore, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, err := os.Stat(s.path); os.IsNotExist(err) {
		return model.DataStore{}, nil
	}
	f, err := os.Open(s.path)
	if err != nil {
		return model.DataStore{}, err
	}
	defer f.Close()

	var ds model.DataStore
	dec := json.NewDecoder(f)
	if err := dec.Decode(&ds); err == nil && ds != nil {
		return ds, nil
	}

	f2, err2 := os.Open(s.path)
	if err2 != nil {
		return model.DataStore{}, nil
	}
	defer f2.Close()

	var old map[string]map[string]model.WatchInfo
	if err := json.NewDecoder(f2).Decode(&old); err != nil || old == nil {
		return model.DataStore{}, nil
	}

	migrated := model.DataStore{}
	for owner, items := range old {
		od := model.OwnerData{
			DefaultIntervalSec: 300,
			Items:              map[string]model.WatchInfo{},
		}
		for uid, wi := range items {
			wi.IntervalSec = 300
			od.Items[uid] = wi
		}
		migrated[owner] = od
	}
	_ = s.Save(migrated)
	return migrated, nil
}

func (s *FileStore) Save(ds model.DataStore) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tmp := s.path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(ds); err != nil {
		f.Close()
		return err
	}
	f.Close()
	return os.Rename(tmp, s.path)
}
