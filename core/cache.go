package core

import (
	"encoding/json"
	"os"
	"strings"

	"github.com/go-redis/redis/v7"
)

type Cache struct {
	r *redis.Client
}

func NewCache(r *redis.Client) *Cache {
	return &Cache{
		r: r,
	}
}

func (c *Cache) Set(v interface{}, keys ...string) error {
	key := strings.Join(keys, ":")
	j, err := json.Marshal(&v)
	if err != nil {
		return err
	}
	errS := c.r.Set(key, j, 0).Err()
	if errS != nil {
		return errS
	}
	return nil
}

func (c *Cache) Get(v interface{}, keys ...string) error {
	key := strings.Join(keys, ":")
	d, err := c.r.Get(key).Result()
	if err == redis.Nil {
		return nil
	}
	if err != nil {
		return err
	}
	errJ := json.Unmarshal([]byte(d), &v)
	if errJ != nil {
		return errJ
	}
	return nil
}

func NewTestRedisClient() *redis.Client {
	redisAddr := os.Getenv("TEST_REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	redisClient := redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: "",
		DB:       1,
	})
	err := redisClient.FlushDB().Err()
	if err != nil {
		panic(err)
	}
	return redisClient
}
