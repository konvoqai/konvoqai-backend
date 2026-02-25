package store

import (
	"database/sql"

	"github.com/redis/go-redis/v9"
)

// Store groups process-wide stateful infrastructure clients.
type Store struct {
	DB    *sql.DB
	Redis *redis.Client
}

func New(db *sql.DB, redisClient *redis.Client) *Store {
	return &Store{
		DB:    db,
		Redis: redisClient,
	}
}

func (s *Store) Close() {
	if s == nil {
		return
	}
	if s.Redis != nil {
		_ = s.Redis.Close()
	}
	if s.DB != nil {
		_ = s.DB.Close()
	}
}
