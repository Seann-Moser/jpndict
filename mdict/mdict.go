package mdict

import "iter"

type Entry struct {
	Key   string
	Value []byte
}

type Reader interface {
	Lookup(key string) ([]byte, error)
	Entries() iter.Seq2[string, []byte]
	Close() error
}
