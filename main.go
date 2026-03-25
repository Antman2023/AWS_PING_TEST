package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"
	"text/tabwriter"
	"time"

	"github.com/go-ping/ping"
)

const (
	ipv4SourceURL       = "http://ec2-reachability.amazonaws.com/prefixes-ipv4.json"
	ipv6SourceURL       = "http://ipv6.ec2-reachability.amazonaws.com/prefixes-ipv6.json"
	defaultCount        = 3
	defaultPacketSize   = 56
	defaultTimeout      = 5 * time.Second
	defaultInterval     = 200 * time.Millisecond
	defaultParallel     = 12
	defaultTop          = 20
	defaultFetchTimeout = 10 * time.Second
	failurePreview      = 10
)

type Config struct {
	Family     string
	Regions    []string
	Count      int
	Timeout    time.Duration
	Interval   time.Duration
	Parallel   int
	Top        int
	Privileged bool
}

type Target struct {
	Region  string
	Prefix  string
	Address string
}

type Result struct {
	Target Target
	Stats  *ping.Statistics
	Err    error
}

func main() {
	log.SetFlags(0)

	cfg, err := parseConfig()
	if err != nil {
		log.Fatal(err)
	}

	sourceURL, err := sourceURLForFamily(cfg.Family)
	if err != nil {
		log.Fatal(err)
	}

	client := &http.Client{Timeout: defaultFetchTimeout}
	targets, err := fetchTargets(client, sourceURL)
	if err != nil {
		log.Fatal(err)
	}

	targets = filterTargets(targets, cfg.Regions)
	if len(targets) == 0 {
		log.Fatal("没有匹配到任何测试目标，请检查 -region 参数")
	}

	results := pingTargets(targets, cfg)
	sortResults(results)
	printResults(os.Stdout, results, cfg)
}

func parseConfig() (Config, error) {
	family := flag.String("family", "ipv4", "地址族，可选 ipv4 或 ipv6")
	region := flag.String("region", "", "按区域过滤，多个区域使用逗号分隔")
	count := flag.Int("count", defaultCount, "每个目标发送的 ping 次数")
	timeout := flag.Duration("timeout", defaultTimeout, "单个目标的总超时时间，例如 3s")
	interval := flag.Duration("interval", defaultInterval, "单个目标的发包间隔，例如 200ms")
	parallel := flag.Int("parallel", defaultParallel, "并发测试的目标数")
	top := flag.Int("top", defaultTop, "仅展示延迟最低的前 N 个可达节点，0 表示全部展示")
	privileged := flag.Bool("privileged", runtime.GOOS == "windows", "是否使用原始套接字模式，Windows 建议保持默认值")

	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "用法: %s [选项]\n\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	cfg := Config{
		Family:     strings.ToLower(strings.TrimSpace(*family)),
		Regions:    parseRegions(*region),
		Count:      *count,
		Timeout:    *timeout,
		Interval:   *interval,
		Parallel:   *parallel,
		Top:        *top,
		Privileged: *privileged,
	}

	switch {
	case cfg.Count < 1:
		return Config{}, errors.New("-count 必须大于 0")
	case cfg.Timeout <= 0:
		return Config{}, errors.New("-timeout 必须大于 0")
	case cfg.Interval <= 0:
		return Config{}, errors.New("-interval 必须大于 0")
	case cfg.Parallel < 1:
		return Config{}, errors.New("-parallel 必须大于 0")
	case cfg.Top < 0:
		return Config{}, errors.New("-top 不能小于 0")
	}

	return cfg, nil
}

func parseRegions(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}

	parts := strings.Split(raw, ",")
	regions := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))

	for _, part := range parts {
		region := strings.ToLower(strings.TrimSpace(part))
		if region == "" {
			continue
		}
		if _, ok := seen[region]; ok {
			continue
		}
		seen[region] = struct{}{}
		regions = append(regions, region)
	}

	return regions
}

func sourceURLForFamily(family string) (string, error) {
	switch family {
	case "ipv4":
		return ipv4SourceURL, nil
	case "ipv6":
		return ipv6SourceURL, nil
	default:
		return "", fmt.Errorf("不支持的地址族 %q，仅支持 ipv4 或 ipv6", family)
	}
}

func fetchTargets(client *http.Client, url string) ([]Target, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("拉取 AWS 测试目标失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("AWS 返回异常状态码: %s", resp.Status)
	}

	var payload []map[string]map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("解析 AWS JSON 失败: %w", err)
	}

	return flattenTargets(payload), nil
}

func flattenTargets(payload []map[string]map[string]string) []Target {
	targets := make([]Target, 0)

	for _, regionEntry := range payload {
		for region, prefixes := range regionEntry {
			for prefix, address := range prefixes {
				targets = append(targets, Target{
					Region:  region,
					Prefix:  prefix,
					Address: address,
				})
			}
		}
	}

	sort.Slice(targets, func(i, j int) bool {
		if targets[i].Region != targets[j].Region {
			return targets[i].Region < targets[j].Region
		}
		if targets[i].Prefix != targets[j].Prefix {
			return targets[i].Prefix < targets[j].Prefix
		}
		return targets[i].Address < targets[j].Address
	})

	return targets
}

func filterTargets(targets []Target, regions []string) []Target {
	if len(regions) == 0 {
		return targets
	}

	allowed := make(map[string]struct{}, len(regions))
	for _, region := range regions {
		allowed[region] = struct{}{}
	}

	filtered := make([]Target, 0, len(targets))
	for _, target := range targets {
		if _, ok := allowed[strings.ToLower(target.Region)]; ok {
			filtered = append(filtered, target)
		}
	}

	return filtered
}

func pingTargets(targets []Target, cfg Config) []Result {
	results := make([]Result, len(targets))
	sem := make(chan struct{}, cfg.Parallel)
	var wg sync.WaitGroup

	for i, target := range targets {
		wg.Add(1)
		sem <- struct{}{}

		go func(index int, target Target) {
			defer wg.Done()
			defer func() { <-sem }()

			results[index] = pingTarget(target, cfg)
		}(i, target)
	}

	wg.Wait()
	return results
}

func pingTarget(target Target, cfg Config) Result {
	pinger, err := ping.NewPinger(target.Address)
	if err != nil {
		return Result{Target: target, Err: fmt.Errorf("创建 pinger 失败: %w", err)}
	}

	pinger.SetPrivileged(cfg.Privileged)
	pinger.Count = cfg.Count
	pinger.Size = defaultPacketSize
	pinger.Timeout = cfg.Timeout
	pinger.Interval = cfg.Interval

	if err := pinger.Run(); err != nil {
		return Result{Target: target, Err: fmt.Errorf("ping 执行失败: %w", err)}
	}

	return Result{Target: target, Stats: pinger.Statistics()}
}

func sortResults(results []Result) {
	sort.Slice(results, func(i, j int) bool {
		leftReachable := results[i].reachable()
		rightReachable := results[j].reachable()

		if leftReachable != rightReachable {
			return leftReachable
		}

		if leftReachable {
			if results[i].Stats.AvgRtt != results[j].Stats.AvgRtt {
				return results[i].Stats.AvgRtt < results[j].Stats.AvgRtt
			}
			if results[i].Stats.PacketLoss != results[j].Stats.PacketLoss {
				return results[i].Stats.PacketLoss < results[j].Stats.PacketLoss
			}
		}

		if results[i].Target.Region != results[j].Target.Region {
			return results[i].Target.Region < results[j].Target.Region
		}
		if results[i].Target.Address != results[j].Target.Address {
			return results[i].Target.Address < results[j].Target.Address
		}
		return results[i].Target.Prefix < results[j].Target.Prefix
	})
}

func printResults(out io.Writer, results []Result, cfg Config) {
	reachable, failed := splitResults(results)

	fmt.Fprintf(out, "已测试 %d 个目标，可达 %d，不可达 %d\n", len(results), len(reachable), len(failed))

	if len(reachable) == 0 {
		fmt.Fprintln(out, "没有可达节点。")
	} else {
		display := reachable
		if cfg.Top > 0 && len(display) > cfg.Top {
			display = display[:cfg.Top]
			fmt.Fprintf(out, "展示延迟最低的前 %d 个可达节点:\n", len(display))
		} else {
			fmt.Fprintln(out, "可达节点列表:")
		}

		tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "Region\tEndpoint\tPrefix\tAvg\tMin\tMax\tLoss")
		for _, result := range display {
			fmt.Fprintf(
				tw,
				"%s\t%s\t%s\t%s\t%s\t%s\t%.0f%%\n",
				result.Target.Region,
				result.Target.Address,
				result.Target.Prefix,
				result.Stats.AvgRtt,
				result.Stats.MinRtt,
				result.Stats.MaxRtt,
				result.Stats.PacketLoss,
			)
		}
		_ = tw.Flush()
	}

	if len(failed) == 0 {
		return
	}

	fmt.Fprintln(out)
	fmt.Fprintf(out, "失败节点预览（共 %d 个）:\n", len(failed))

	preview := failed
	if len(preview) > failurePreview {
		preview = preview[:failurePreview]
		fmt.Fprintf(out, "仅展示前 %d 个失败节点，按区域和地址排序。\n", len(preview))
	}

	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "Region\tEndpoint\tPrefix\tReason")
	for _, result := range preview {
		fmt.Fprintf(
			tw,
			"%s\t%s\t%s\t%s\n",
			result.Target.Region,
			result.Target.Address,
			result.Target.Prefix,
			result.reason(),
		)
	}
	_ = tw.Flush()
}

func splitResults(results []Result) ([]Result, []Result) {
	reachable := make([]Result, 0, len(results))
	failed := make([]Result, 0, len(results))

	for _, result := range results {
		if result.reachable() {
			reachable = append(reachable, result)
			continue
		}
		failed = append(failed, result)
	}

	return reachable, failed
}

func (r Result) reachable() bool {
	return r.Err == nil && r.Stats != nil && r.Stats.PacketsRecv > 0
}

func (r Result) reason() string {
	if r.Err != nil {
		return r.Err.Error()
	}
	if r.Stats == nil {
		return "没有统计信息"
	}
	if r.Stats.PacketsRecv == 0 {
		return "100% 丢包"
	}
	return ""
}
