package db

import (
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/iterator"
)

// LevelDB wraps the actual LevelDB connection
type LevelDB struct {
	conn *leveldb.DB
}

// NewLevelDB opens (or creates) a LevelDB instance at the given path
func NewLevelDB(path string) (*LevelDB, error) {
	db, err := leveldb.OpenFile(path, nil)
	if err != nil {
		return nil, err
	}
	return &LevelDB{conn: db}, nil
}

// Close safely closes the LevelDB connection
func (l *LevelDB) Close() error {
	return l.conn.Close()
}

// Put inserts or updates a key-value pair
func (l *LevelDB) Put(key, value []byte) error {
	return l.conn.Put(key, value, nil)
}

// Get retrieves the value for a given key
func (l *LevelDB) Get(key []byte) ([]byte, error) {
	return l.conn.Get(key, nil)
}

// NewIterator returns an iterator to loop over all key-value pairs
func (l *LevelDB) NewIterator() iterator.Iterator {
	return l.conn.NewIterator(nil, nil)
}


