package crack

import (
	"fmt"
	cmap "github.com/orcaman/concurrent-map/v2"
	"strings"
	"sync"
	"time"

	"crack-modify/pkg/crack/plugins"

	"github.com/cheggaaa/pb/v3"
	"github.com/projectdiscovery/gologger"
)

type Options struct {
	Threads  int
	Timeout  int
	Delay    int
	CrackAll bool
	Spray    bool // 喷洒模式: 对所有用户各试一个口令再换下一个(防账户锁定), 与默认的字典爆破(每用户跑全口令)顺序相反

	UserMap      map[string][]string
	CommonPass   []string
	TemplatePass []string
}

type Runner struct {
	options *Options
}

func NewRunner(options *Options) (*Runner, error) {
	// 边界值兜底, 避免非法参数导致死锁或未定义行为:
	//   Threads < 1 会让 taskChan 容量为 0 且不启动 worker → 生产者入队死锁
	//   Threads 过大会瞬间开出海量 goroutine
	//   Timeout < 1 会让 net.DialTimeout(...,0) 行为未定义
	//   Delay  为负无意义
	if options.Threads < 1 {
		options.Threads = 1
	}
	if options.Threads > 1000 {
		options.Threads = 1000
	}
	if options.Timeout < 1 {
		options.Timeout = 10
	}
	if options.Delay < 0 {
		options.Delay = 0
	}
	if len(options.UserMap) == 0 {
		options.UserMap = userMap
	}
	if len(options.CommonPass) == 0 {
		options.CommonPass = commonPass
	}
	if len(options.TemplatePass) == 0 {
		options.TemplatePass = templatePass
	}
	return &Runner{
		options: options,
	}, nil
}

type Result struct {
	Addr     string
	Protocol string
	UserPass string
}

type IpAddr struct {
	Ip       string
	Port     int
	Protocol string
}

func (r *Runner) Run(addrs []*IpAddr, userDict []string, passDict []string) (results []*Result) {
	for _, addr := range addrs {
		results = append(results, r.Crack(addr, userDict, passDict)...)
	}
	return
}

func (r *Runner) Crack(addr *IpAddr, userDict []string, passDict []string) (results []*Result) {
	gologger.Info().Msgf("开始爆破: %v:%v %v", addr.Ip, addr.Port, addr.Protocol)

	var tasks []plugins.Service
	// P3: 去重改用字符串 key(无需密码学哈希), \x00 分隔避免 ("ab","c") 与 ("a","bc") 碰撞
	taskSet := map[string]struct{}{}
	// GenTask
	if len(userDict) == 0 {
		userDict = r.options.UserMap[addr.Protocol]
	}
	if len(passDict) == 0 {
		passDict = append(passDict, r.options.TemplatePass...)
		passDict = append(passDict, r.options.CommonPass...)
	}
	// 生成任务:
	//   字典爆破(默认): 对每个用户跑全部口令  for user { for pass }
	//   喷洒模式(Spray): 对所有用户各试一个口令再换下一个  for pass { for user }
	//     喷洒模式配合 --delay 可降低单用户被锁定风险(同一用户两次尝试间隔 = 用户数*delay)
	addTask := func(user, pass string) {
		pass = strings.ReplaceAll(pass, "{user}", user)
		dedupKey := user + "\x00" + pass
		if _, ok := taskSet[dedupKey]; ok {
			return
		}
		taskSet[dedupKey] = struct{}{}
		tasks = append(tasks, plugins.Service{
			Ip:       addr.Ip,
			Port:     addr.Port,
			Protocol: addr.Protocol,
			User:     user,
			Pass:     pass,
			Timeout:  r.options.Timeout,
		})
	}
	if r.options.Spray {
		for _, pass := range passDict {
			for _, user := range userDict {
				addTask(user, pass)
			}
		}
	} else {
		for _, user := range userDict {
			for _, pass := range passDict {
				addTask(user, pass)
			}
		}
	}
	// RunTask
	// P3: stopMap 直接以 addrStr 作 key, 省去每任务一次 MD5
	addrStr := fmt.Sprintf("%v:%v", addr.Ip, addr.Port)
	stopMap := cmap.New[string]()
	mutex := &sync.Mutex{}
	wg := &sync.WaitGroup{}
	taskChan := make(chan plugins.Service, r.options.Threads)

	// P1: 全局限速 gate。Delay>0 时, 所有 worker 共享一个 ticker,
	// 每次请求前取一个令牌 → 整体速率被限制为 1 req / Delay 秒(真正的"请求间隔"语义),
	// 而非旧的每 worker 各自 sleep(实际速率 = Threads/Delay)。
	var gate <-chan time.Time
	if r.options.Delay > 0 {
		ticker := time.NewTicker(time.Duration(r.options.Delay) * time.Second)
		defer ticker.Stop()
		gate = ticker.C
	}

	bar := pb.StartNew(len(tasks))
	for i := 0; i < r.options.Threads; i++ {
		go func() {
			for task := range taskChan {
				userPass := fmt.Sprintf("%v:%v", task.User, task.Pass)
				// 判断是否已经停止爆破
				if stopMap.Has(addrStr) {
					bar.Increment() // B5: 进度按"已完成"计, 含被跳过的任务
					wg.Done()
					continue
				}
				// P1: 限速 — 等待全局令牌(在 scanFunc 之前, 保证请求间隔)
				if gate != nil {
					<-gate
				}
				gologger.Debug().Msgf("[trying] %v", userPass)
				scanFunc := plugins.ScanFuncMap[task.Protocol]
				resp, err := scanFunc(&task)
				switch resp {
				case plugins.CrackSuccess:
					if !r.options.CrackAll {
						stopMap.Set(addrStr, "ok")
					}
					gologger.Silent().Msgf("%v -> %v %v", addr.Protocol, addrStr, userPass)
					mutex.Lock()
					results = append(results, &Result{
						Addr:     addrStr,
						Protocol: addr.Protocol,
						UserPass: userPass,
					})
					mutex.Unlock()
				case plugins.CrackError:
					stopMap.Set(addrStr, "ok")
					gologger.Debug().Msgf("crack err, %v", err)
				case plugins.CrackFail:
				}
				bar.Increment() // B5: 进度按 worker 完成计, 而非生产者入队
				wg.Done()
			}
		}()
	}

	// B5: 生产者只负责入队, 不再 Increment(进度条移到 worker 完成处)
	for _, task := range tasks {
		wg.Add(1)
		taskChan <- task
	}
	close(taskChan)
	wg.Wait()
	bar.Finish()

	gologger.Info().Msgf("爆破结束")

	return
}
