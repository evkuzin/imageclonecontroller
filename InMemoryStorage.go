package main

import (
	"sync"
)

type inMemoryStorage struct {
	Mutex  *sync.Mutex
	Images map[string]string
}

func newInMemoryStorage() *inMemoryStorage {
	return &inMemoryStorage{
		Mutex:  &sync.Mutex{},
		Images: map[string]string{},
	}
}

func (s *inMemoryStorage) CheckImage(image string) (string, bool) {
	s.Mutex.Lock()
	defer s.Mutex.Unlock()
	if img, ok := s.Images[image]; ok {
		return img, true
	}
	return "", false
}

func (s *inMemoryStorage) PutImage(old, new string) {
	s.Mutex.Lock()
	s.Images[old] = new
	s.Mutex.Unlock()
}
