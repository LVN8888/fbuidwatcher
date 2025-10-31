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

func NewFileStore(path string) *FileStore {
	return &FileStore{path: path}
}

func (s *FileStore) Load() (model.DataStore, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := os.Open(s.path)
	if err != nil {
		return model.DataStore{}, err
	}
	defer f.Close()
	var ds model.DataStore
	if err := json.NewDecoder(f).Decode(&ds); err != nil {
		return model.DataStore{}, nil
	}
	return ds, nil
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
