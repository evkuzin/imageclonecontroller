package main

import (
	"reflect"
	"sync"
	"testing"
)

func TestInMemoryStorage_CheckImage(t *testing.T) {
	type fields struct {
		Mutex  *sync.Mutex
		Images map[string]string
	}
	type args struct {
		image string
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   string
		want1  bool
	}{
		{
			"Check nonexistent image",
			fields{&sync.Mutex{}, map[string]string{}},
			args{"nginx"},
			"",
			false,
		},
		{
			"Check existent image",
			fields{&sync.Mutex{}, map[string]string{
				"nginx": "nginx2",
			}},
			args{"nginx"},
			"nginx2",
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &inMemoryStorage{
				Mutex:  tt.fields.Mutex,
				Images: tt.fields.Images,
			}
			got, got1 := s.CheckImage(tt.args.image)
			if got != tt.want {
				t.Errorf("CheckImage() got = %v, want %v", got, tt.want)
			}
			if got1 != tt.want1 {
				t.Errorf("CheckImage() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}

func TestInMemoryStorage_PutImage(t *testing.T) {
	type fields struct {
		Mutex  *sync.Mutex
		Images map[string]string
	}
	type args struct {
		old string
		new string
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   map[string]string
	}{
		{
			"Put image",
			fields{&sync.Mutex{}, map[string]string{
				"nginx": "nginx2",
			},
			},
			args{
				old: "test",
				new: "test1",
			},
			map[string]string{
				"nginx": "nginx2",
				"test":  "test1",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &inMemoryStorage{
				Mutex:  tt.fields.Mutex,
				Images: tt.fields.Images,
			}
			s.PutImage(tt.args.old, tt.args.new)
			if !reflect.DeepEqual(s.Images, tt.want) {
				t.Errorf("CheckImage() got = %#v, want %#v", s.Images, tt.want)
			}
		})
	}
}

func TestNewInMemoryStorage(t *testing.T) {
	tests := []struct {
		name string
		want *inMemoryStorage
	}{
		{
			"Create in memory storage",
			&inMemoryStorage{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := newInMemoryStorage(); reflect.TypeOf(got) != reflect.TypeOf(&inMemoryStorage{}) {
				t.Errorf("newInMemoryStorage() = %v, want %v", got, tt.want)
			}
		})
	}
}
