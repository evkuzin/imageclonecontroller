package main

import "sync"

type InMemoryStorage struct {
    Mutex  *sync.Mutex
    Images map[string]string
}

func NewInMemoryStorage() *InMemoryStorage {
    return &InMemoryStorage{
        Mutex:  &sync.Mutex{},
        Images: map[string]string{},
    }
}

func (s *InMemoryStorage) CheckImage(image string) (string, bool) {
    s.Mutex.Lock()
    defer s.Mutex.Unlock()
    if img, ok := s.Images[image]; ok {
        return img, true
    } else {
        return img, false
    }
}

func (s *InMemoryStorage) PutImage(old, new string) {
    s.Mutex.Lock()
    s.Images[old] = new
    s.Mutex.Unlock()
}
