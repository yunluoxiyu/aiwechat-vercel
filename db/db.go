package db

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/go-redis/redis/v8"
)

var (
	ChatDbInstance ChatDb        = nil
	RedisClient    *redis.Client = nil
	Cache          sync.Map
)

const (
	PROMPT_KEY = "prompt"
	MSG_KEY    = "msg"
)

func init() {
	db, err := GetChatDb()
	if err != nil {
		fmt.Println(err)
		return
	}
	ChatDbInstance = db
}

type Msg struct {
	Role string
	Msg  string
}

type ChatDb interface {
	GetMsgList(botType string, userId string) ([]Msg, error)
	SetMsgList(botType string, userId string, msgList []Msg)
}

type RedisChatDb struct {
	client *redis.Client
}

func NewRedisChatDb(url string) (*RedisChatDb, error) {
	options, err := redis.ParseURL(url)
	if err != nil {
		fmt.Println(err)
		return nil, errors.New("redis url error")
	}
	options.TLSConfig = &tls.Config{InsecureSkipVerify: true}
	client := redis.NewClient(options)
	RedisClient = client
	return &RedisChatDb{
		client: client,
	}, nil
}

func (r *RedisChatDb) GetMsgList(botType string, userId string) ([]Msg, error) {
	result, err := r.client.Get(context.Background(), fmt.Sprintf("%v:%v:%v", MSG_KEY, botType, userId)).Result()
	if err != nil {
		return nil, err
	}
	var msgList []Msg
	err = sonic.Unmarshal([]byte(result), &msgList)
	if err != nil {
		return nil, err
	}
	return msgList, nil
}

func (r *RedisChatDb) SetMsgList(botType string, userId string, msgList []Msg) {
	res, err := sonic.Marshal(msgList)
	if err != nil {
		fmt.Println(err)
		return
	}
	r.client.Set(context.Background(), fmt.Sprintf("%v:%v:%v", MSG_KEY, botType, userId), res, time.Minute*30)
}

func GetChatDb() (ChatDb, error) {
	kvUrl := os.Getenv("KV_URL")
	if kvUrl == "" {
		return nil, errors.New("请配置KV_URL")
	} else {
		db, err := NewRedisChatDb(kvUrl)
		if err != nil {
			return nil, err
		}
		return db, nil
	}
}

func GetValueWithMemory(key string) (string, bool) {
	value, ok := Cache.Load(key)
	if ok {
		return value.(string), ok
	}
	return "", false
}

func SetValueWithMemory(key string, val any) {
	Cache.Store(key, val)
}

func DeleteKeyWithMemory(key string) {
	Cache.Delete(key)
}

func GetValue(key string) (val string, err error) {
	val, flag := GetValueWithMemory(key)
	if !flag {
		if RedisClient == nil {
			return
		}
		val, err = RedisClient.Get(context.Background(), key).Result()
		SetValueWithMemory(key, val)
		return
	}
	return
}

func SetValue(key string, val any, expires time.Duration) (err error) {
	SetValueWithMemory(key, val)

	if RedisClient == nil {
		return
	}
	if expires == 0 {
		expires = time.Minute * 30
	}

	err = RedisClient.Set(context.Background(), key, val, expires).Err()

	return
}

func DeleteKey(key string) {
	DeleteKeyWithMemory(key)
	if RedisClient == nil {
		return
	}
	RedisClient.Del(context.Background(), key)
}

func SetPrompt(userId, botType, prompt string) {
	SetValue(fmt.Sprintf("%s:%s:%s", PROMPT_KEY, userId, botType), prompt, 0)
}

func GetPrompt(userId, botType string) (string, error) {
	return GetValue(fmt.Sprintf("%s:%s:%s", PROMPT_KEY, userId, botType))
}

func RemovePrompt(userId, botType string) {
	DeleteKey(fmt.Sprintf("%s:%s:%s", PROMPT_KEY, userId, botType))
}
