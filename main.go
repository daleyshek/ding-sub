package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/go-redis/redis"
)

// ConfigFileName 配置文件名称
const ConfigFileName = "app.json"

// AppConf 配置参数,redis 数据库、频道和钉钉群聊机器人webhook链接
type AppConf struct {
	Subscribers []Subscriber `json:"subscribers"`
	Redis       RedisConf    `json:"redis"`
	modTime     time.Time
}

// Subscriber 订阅
type Subscriber struct {
	RedisChannel   string `json:"channel"`
	DingHookBotURL string `json:"hookUrl"`
}

// RedisConf Redis配置
type RedisConf struct {
	Password string `json:"password"`
	Addr     string `json:"address"`
}

var c AppConf

var r *redis.Client

func main() {
	var wg sync.WaitGroup
	wg.Add(len(c.Subscribers))
	for _, sb := range c.Subscribers {
		go subscribe(sb, &wg)
	}
	// select {}
	wg.Wait()
}

func init() {
	c.Subscribers = append(c.Subscribers, Subscriber{
		RedisChannel:   "",
		DingHookBotURL: "",
	})
	c.Redis.Addr = "127.0.0.1:6379"
	loadConf(true)
}

// loadConf 加载配置文件
func loadConf(initial bool) bool {
	f, err := os.Open(ConfigFileName)
	if err != nil {
		log.Println("未在程序目录找到配置文件")
		// 创建初始配置文件
		f, err = os.OpenFile(ConfigFileName, os.O_CREATE|os.O_RDWR, 0666)
		if err == nil {
			c.modTime = time.Now()
			b, _ := json.Marshal(&c)
			f.Write(b)
			f.Close()
			log.Println("配置文件已生成，请填写配置参数")
		}
		return false
	}
	defer f.Close()
	fi, _ := f.Stat()
	if fi.ModTime() != c.modTime {
		// s配置文件发生了变更或者首次加载
		content, _ := ioutil.ReadAll(f)
		err = json.Unmarshal(content, &c)
		c.modTime = fi.ModTime() //重置修改时间
		if err != nil && initial {
			log.Println("加载配置文件失败")
			return false
		}
		// 更新redis
		r = redis.NewClient(&redis.Options{
			Addr:     c.Redis.Addr,
			Password: c.Redis.Password,
		})
	}

	// 如果是首次加载，则开启配置更改监听
	if initial {
		go func() {
			for {
				time.Sleep(time.Minute * 5)
				loadConf(false)
			}
		}()
	}
	return true
}

// confWatch 监听配置改动
func confWatch() bool {
	f, err := os.Open(ConfigFileName)
	if err != nil {
		log.Println("未在程序目录找到配置文件")
		return false
	}
	defer f.Close()
	stat, _ := f.Stat()
	if stat.ModTime() != c.modTime {
		if loadConf(false) {
			log.Println("成功更新配置")
			return true
		}
	}
	return false
}

// subscribe 订阅
func subscribe(sb Subscriber, wg *sync.WaitGroup) {
	if c.Redis.Addr == "" || sb.RedisChannel == "" || sb.DingHookBotURL == "" {
		log.Println("请先配置app.conf参数")
		wg.Done()
		return
	}
	subpub := r.Subscribe(sb.RedisChannel)

	ch := subpub.Channel()
	for c := range ch {
		fmt.Println(c.Payload)
		sendDingNotify(sb, c.Payload)
	}
}

// dingMSG 钉钉消息结构
type dingMSG struct {
	MSGType string `json:"msgtype"`
	Text    struct {
		Content string `json:"content"`
	} `json:"text"`
}

// sendDingNotify 发送钉钉消息
func sendDingNotify(sb Subscriber, msg string) {
	var m dingMSG
	m.MSGType = "text"
	m.Text.Content = msg
	b, _ := json.Marshal(&m)
	resp, err := http.Post(sb.DingHookBotURL, "application/json", bytes.NewReader(b))
	if err == nil {
		resp.Body.Close()
	}
}
