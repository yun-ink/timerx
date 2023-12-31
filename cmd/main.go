package main

import (
	"context"
	"fmt"
	"time"

	"code.yun.ink/open/timer"
	"github.com/go-redis/redis/v8"
)

func main() {
	// m := make(map[string]time.Time)
	// m["sss"] = time.Now()

	// b, _ := json.Marshal(m)

	// fmt.Println(string(b))

	// mm := make(map[string]time.Time)
	// json.Unmarshal(b, &mm)

	// fmt.Println(mm)

	// re()
	// d()
	worker()

}

func worker() {
	client := getRedis()
	w := timer.InitWorker(context.Background(), client, &Worker{})
	w.Add("test", "test", 1*time.Second, map[string]interface{}{
		"test": "test",
	})
	w.Add("test2", "test", 1*time.Second, map[string]interface{}{
		"test": "test",
	})
	w.Add("test3", "test", 1*time.Second, map[string]interface{}{
		"test": "test",
	})
	w.Add("test4", "test", 1*time.Second, map[string]interface{}{
		"test": "test",
	})
	w.Add("test5", "test", 1*time.Second, map[string]interface{}{
		"test": "test",
	})

	select {}
}

type Worker struct{}

func (w *Worker) Worker(uniqueKey string, jobType string,data map[string]interface{}) timer.WorkerCode {
	fmt.Println("执行时间:", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Println(uniqueKey, jobType)
	fmt.Println(data)
	return timer.WorkerCodeAgain
}

func getRedis() *redis.Client {
	client := redis.NewClient(&redis.Options{
		Addr:     "127.0.0.1" + ":" + "6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	})
	if client == nil {
		panic("redis init error")
	}
	return client
}

func re() {

	client := getRedis()

	ctx := context.Background()
	cl := timer.InitCluster(ctx, client)
	cl.AddTimer(ctx, "test1", 1*time.Millisecond, aa, timer.ExtendParams{
		Params: map[string]interface{}{
			"test": "text1",
		},
	})
	cl.AddTimer(ctx, "test2", 1*time.Millisecond, aa, timer.ExtendParams{
		Params: map[string]interface{}{
			"test": "text2",
		},
	})
	cl.AddTimer(ctx, "test3", 1*time.Millisecond, aa, timer.ExtendParams{
		Params: map[string]interface{}{
			"test": "text3",
		},
	})
	cl.AddTimer(ctx, "test4", 1*time.Millisecond, aa, timer.ExtendParams{
		Params: map[string]interface{}{
			"test": "text4",
		},
	})
	cl.AddTimer(ctx, "test5", 1*time.Millisecond, aa, timer.ExtendParams{
		Params: map[string]interface{}{
			"test": "text5",
		},
	})
	cl.AddTimer(ctx, "test6", 1*time.Millisecond, aa, timer.ExtendParams{
		Params: map[string]interface{}{
			"test": "text6",
		},
	})
	cl.AddTimer(ctx, "test7", 1*time.Millisecond, aa, timer.ExtendParams{
		Params: map[string]interface{}{
			"test": "text7",
		},
	})
	cl.AddTimer(ctx, "test8", 1*time.Millisecond, aa, timer.ExtendParams{
		Params: map[string]interface{}{
			"test": "text8",
		},
	})
	cl.AddTimer(ctx, "test9", 1*time.Millisecond, aa, timer.ExtendParams{
		Params: map[string]interface{}{
			"test": "text9",
		},
	})

	select {}
}

func aa(ctx context.Context) bool {
	// fmt.Println(time.Now().Format(time.RFC3339))
	// fmt.Println("gggggggggggggggggggggggggggg")
	a, err := timer.GetExtendParams(ctx)
	fmt.Printf("%+v %+v \n\n", a, err)
	time.Sleep(time.Second * 5)
	return true
}

func d() {

	client := redis.NewClient(&redis.Options{
		Addr:     "127.0.0.1" + ":" + "6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	})
	if client == nil {
		fmt.Println("redis init error")
		return
	}

	client.ZAdd(context.Background(), "lockx:test2", &redis.Z{
		Score:  50,
		Member: "test",
	})

	script := `
	local token = redis.call('zrangebyscore',KEYS[1],ARGV[1],ARGV[2])
	for i,v in ipairs(token) do
		redis.call('zrem',KEYS[1],v)
		redis.call('lpush',KEYS[2],v)
	end
	return "OK"
	`
	res, err := client.Eval(context.Background(), script, []string{"lockx:test2", "lockx:push"}, 0, 100).Result()
	fmt.Println(res, err)

	for i := 0; i < 10; i++ {
		l, e := client.RPop(context.Background(), "lockx:push").Result()
		fmt.Println(l, e)
	}

}
