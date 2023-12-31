package timer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"runtime/debug"
	"sync"
	"time"

	"code.yun.ink/open/timer/lockx"
	"github.com/go-redis/redis/v8"
)

// 单例模式
var clusterOnceLimit sync.Once

// 已注册的任务列表
var clusterWorkerList sync.Map

type cluster struct {
	ctx     context.Context
	redis   *redis.Client
	lockKey string // 全局计算的key
	nextKey string // 下一次执行的key
	zsetKey string // 有序集合的key
	listKey string // 可执行的任务列表的key
}

var clu *cluster = nil

func InitCluster(ctx context.Context, red *redis.Client) *cluster {
	clusterOnceLimit.Do(func() {
		clu = &cluster{
			ctx:     ctx,
			redis:   red,
			lockKey: "timer:cluster_globalLockKey",
			nextKey: "timer:cluster_nextKey",
			zsetKey: "timer:cluster_zsetKey",
			listKey: "timer:cluster_listKey",
		}

		// 监听任务
		go clu.watch()

		timer := time.NewTicker(time.Millisecond * 200)

		go func(ctx context.Context, red *redis.Client) {
		Loop:
			for {
				select {
				case <-timer.C:
					clu.getTask()
					clu.getNextTime()
				case <-ctx.Done():
					break Loop
				}
			}
		}(ctx, red)
	})
	return clu
}

func (c *cluster) AddTimer(ctx context.Context, uniqueKey string, spaceTime time.Duration, callback callback, extend ExtendParams) error {
	_, ok := clusterWorkerList.Load(uniqueKey)
	if ok {
		return errors.New("key已存在")
	}

	if spaceTime != spaceTime.Abs() {
		return errors.New("时间间隔不能为负数")
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	lock := lockx.NewGlobalLock(ctx, c.redis, uniqueKey)
	tB := lock.Try(10)
	if !tB {
		return errors.New("添加失败")
	}
	defer lock.Unlock()

	nowTime := time.Now()

	t := timerStr{
		BeginTime: nowTime,
		NextTime:  nowTime,
		SpaceTime: spaceTime,
		Callback:  callback,
		Extend:    extend,
		UniqueKey: uniqueKey,
	}

	clusterWorkerList.Store(uniqueKey, t)

	cacheStr, _ := c.redis.Get(ctx, c.nextKey).Result()
	execTime := make(map[string]time.Time)
	json.Unmarshal([]byte(cacheStr), &execTime)

	p := c.redis.Pipeline()

	p.ZAdd(ctx, c.zsetKey, &redis.Z{
		Score:  float64(nextTime.UnixMilli()),
		Member: uniqueKey,
	})
	execTime[uniqueKey] = nextTime
	n, _ := json.Marshal(execTime)
	// fmt.Println("execTime:", execTime, string(n))
	p.Set(ctx, c.nextKey, string(n), 0)

	_, err := p.Exec(ctx)

	// fmt.Println("添加", err)

	return err
}

// 计算下一次执行的时间
func (c *cluster) getNextTime() {

	// log.Println("begin computer")
	ctx, cancel := context.WithCancel(c.ctx)
	defer cancel()

	lock := lockx.NewGlobalLock(ctx, c.redis, c.lockKey)
	// 获取锁
	lockBool := lock.Lock()
	if !lockBool {
		// log.Println("timer:获取锁失败")
		return
	}
	defer lock.Unlock()

	// 计算下一次时间

	// 读取执行的缓存
	cacheStr, _ := c.redis.Get(ctx, c.nextKey).Result()
	execTime := make(map[string]time.Time)
	json.Unmarshal([]byte(cacheStr), &execTime)

	p := c.redis.Pipeline()

	nowTime := time.Now()

	clusterWorkerList.Range(func(key, value interface{}) bool {
		val := value.(timerStr)
		beforeTime := execTime[val.UniqueKey]
		if beforeTime.After(nowTime) {
			return true
		}
		nextTime := getNextExecTime(beforeTime, val.SpaceTime)
		execTime[val.UniqueKey] = nextTime

		p.ZAdd(ctx, c.zsetKey, &redis.Z{
			Score:  float64(nextTime.UnixMilli()),
			Member: val.UniqueKey,
		})
		// log.Println("computeTime add", c.zsetKey, val.UniqueKey, nextTime.UnixMilli())
		return true
	})

	// 更新缓存
	b, _ := json.Marshal(execTime)
	p.Set(ctx, c.nextKey, string(b), 0)

	_, err := p.Exec(ctx)
	_ = err
}

// 递归遍历获取执行时间
func getNextExecTime(beforeTime time.Time, spaceTime time.Duration) time.Time {
	nowTime := time.Now()
	if beforeTime.After(nowTime) {
		return beforeTime
	}
	nextTime := beforeTime.Add(spaceTime)
	if nextTime.Before(nowTime) {
		nextTime = getNextExecTime(nextTime, spaceTime)
	}
	return nextTime
}

// 获取任务
func (c *cluster) getTask() {
	// 定时去Redis获取任务
	// zb := redis.ZRangeBy{
	// 	Min: "0",
	// 	Max: fmt.Sprintf("%+v", time.Now().UnixMilli()),
	// }

	// taskList, _ := c.redis.ZRangeByScore(c.ctx, c.zsetKey, &zb).Result()

	// if len(taskList) == 0 {
	// 	return
	// }

	// p := c.redis.Pipeline()

	// for _, val := range taskList {
	// 	// 添加到可执行队列
	// 	p.LPush(c.ctx, c.listKey, val)
	// 	// 删除有序集合
	// 	p.ZRem(c.ctx, c.zsetKey, val)
	// }
	// _, err := p.Exec(c.ctx)
	// // fmt.Println(err)
	// _ = err

	script := `
	local token = redis.call('zrangebyscore',KEYS[1],ARGV[1],ARGV[2])
	for i,v in ipairs(token) do
		redis.call('zrem',KEYS[1],v)
		redis.call('lpush',KEYS[2],v)
	end
	return "OK"
	`
	c.redis.Eval(c.ctx, script, []string{c.zsetKey, c.listKey}, 0, time.Now().UnixMilli()).Result()

}

// 监听任务
func (c *cluster) watch() {
	// 执行任务
	for {
		keys, err := c.redis.BLPop(c.ctx, time.Second*10, c.listKey).Result()
		if err != nil {
			fmt.Println("watch err:", err)
			continue
		}
		go doTask(c.ctx, c.redis, keys[1])
	}
}

// 执行任务
func doTask(ctx context.Context, red *redis.Client, taskId string) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	defer func() {
		if err := recover(); err != nil {
			fmt.Println("timer:定时器出错", err)
			log.Println("errStack", string(debug.Stack()))
		}
	}()

	val, ok := clusterWorkerList.Load(taskId)
	if !ok {
		fmt.Println("doTask timer:任务不存在", taskId)
		return
	}
	t := val.(timerStr)

	// 这里加一个全局锁
	lock := lockx.NewGlobalLock(ctx, red, taskId)
	tB := lock.Lock()
	if !tB {
		fmt.Println("doTask timer:获取锁失败", taskId)
		return
	}
	defer lock.Unlock()

	ctx = context.WithValue(ctx, extendParamKey, t.Extend)

	// 执行任务
	t.Callback(ctx)
}
