package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/cloudflare/cloudflare-go/v4"
	"github.com/cloudflare/cloudflare-go/v4/dns"
	"github.com/cloudflare/cloudflare-go/v4/option"
	"github.com/cloudflare/cloudflare-go/v4/zones"
	"github.com/vishvananda/netlink"
	"golang.org/x/exp/slices"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Config struct {
	LocalIpv4AddrApiUrl string                   `yaml:"localIpv4AddrApiUrl"`
	LocalIpv6AddrApiUrl string                   `yaml:"localIpv6AddrApiUrl"`
	UniqueToken         string                   `json:"uniqueToken"`
	CloudflareEmail     string                   `json:"cloudflareEmail"`
	CloudflareApiKey    string                   `json:"cloudflareApiKey"`
	DomainName          string                   `json:"domainName"`
	RecordMap           map[string][]LocalRecord `json:"recordMap"`
}

type LocalRecord struct {
	Name    string
	Comment string
}

var config *Config

func main() {
	// 获取配置文件
	var err error
	config, err = loadConfig()
	if err != nil {
		fmt.Println("第一次运行请在运行目录下创建config.json配置文件")
		fmt.Println("注：mac填00:00:00:00:00:00就是自己，uniqueToken同域名每个机子ddns要不一样不然会互删配置内容")
		fmt.Println("举例：")
		fmt.Println(`{
  "uniqueToken": "token1111",
  "cloudflareEmail": "your@domain.com",
  "cloudflareApiKey": "your-api-key",
  "domainName": "your-domain-name",
  "localIpv4AddrApiUrl": "https://api-ipv4.ip.sb/ip",
  "localIpv6AddrApiUrl": "https://api-ipv6.ip.sb/ip",
  "recordMap": {
    "00:00:00:00:00:00": [
      {
        "name": "virt210",
        "comment": ""
      }
    ],
    "bc:24:11:42:15:81": [
      {
        "name": "virt211",
        "comment": ""
      },
      {
        "name": "xiaoyan",
        "comment": ""
      }
    ]
  }
}`)
		panic(err)
	}

	// 所有本地出现过的ip，用于移除失效的ip避免堆积
	localIpList := make([]string, 0)

	// 本代码应该在路由器linux中执行，一般电脑不会主动扫描
	// linkIndex 0 代表查找所有网络接口
	neighs, err := netlink.NeighList(0, netlink.FAMILY_V6)
	if err != nil {
		panic(err)
	}

	var hasNudTestDo = false
	filtered := make([]netlink.Neigh, 0, len(neighs))
	for _, n := range neighs {
		if n.HardwareAddr != nil &&
			config.RecordMap[n.HardwareAddr.String()] != nil {
			hasNudTestDo = true
			ipv6NudTest(n)
		}
	}

	// 等待邻居状态更新，变为FAILED后直接清掉了，不再会浪费时间等待了
	if hasNudTestDo {
		time.Sleep(3 * time.Second)
		// NUD_PROBE 探测完毕后再次获取，只要REACHABLE的ip
		neighs, err = netlink.NeighList(0, netlink.FAMILY_V6)
		if err != nil {
			panic(err)
		}
	}

	for _, n := range neighs {
		if n.HardwareAddr != nil &&
			config.RecordMap[n.HardwareAddr.String()] != nil &&
			isValidReachableGlobalIPv6(n) {
			filtered = append(filtered, n)
		}
	}

	neighs = filtered

	// 数据包装成map
	var macIpv6Map = make(map[string][]string)

	fmt.Println("\u001B[32m====================本机ipv6(公网)列表====================\u001B[0m")
	for _, n := range neighs {
		if n.HardwareAddr.String() == "00:00:00:00:00:00" {
			continue // 本机ip 单独处理
		}
		macIpv6Map[n.HardwareAddr.String()] = append(macIpv6Map[n.HardwareAddr.String()], n.IP.String())
		localIpList = append(localIpList, n.IP.String())
	}

	// 本机ipv6获取
	if config.RecordMap["00:00:00:00:00:00"] != nil {
		myIpv6 := getLocalIpv6ByHttp()
		if len(myIpv6) > 0 {
			macIpv6Map["00:00:00:00:00:00"] = append(macIpv6Map["00:00:00:00:00:00"], myIpv6)
			localIpList = append(localIpList, myIpv6)
		}

	}

	for mac, ipList := range macIpv6Map {
		// MAC 00:00:00:00:00:00 为本机，使用第三方http api服务获取公网
		localRecords := config.RecordMap[mac]
		if localRecords != nil && len(localRecords) > 0 {
			fmt.Print("受影响的域名：")
			for lId, localRecord := range localRecords {
				fmt.Printf("%s.%s", localRecord.Name, config.DomainName)
				if lId+1 != len(localRecords) {
					fmt.Print("、")
				}
			}
			fmt.Printf("\nMAC: %s\nipv6列表：\n", mac)
			for _, ip := range ipList {
				fmt.Println(ip)
			}
			fmt.Println()
		}
	}

	// cloudflare golang客户端
	client := cloudflare.NewClient(
		option.WithAPIKey(config.CloudflareApiKey),  // defaults to os.LookupEnv("CLOUDFLARE_API_KEY")
		option.WithAPIEmail(config.CloudflareEmail), // defaults to os.LookupEnv("CLOUDFLARE_EMAIL")
	)

	// 获取域名id
	response, err := client.Zones.List(context.TODO(), zones.ZoneListParams{Name: cloudflare.F(config.DomainName)})
	if err != nil {
		panic(err.Error())
	}

	zoneId := response.Result[0].ID
	fmt.Printf("\n主域名：%s\n", config.DomainName)
	fmt.Printf("zoneId: %s\n", zoneId)

	cfRecordListResponse, err := client.DNS.Records.List(context.TODO(), dns.RecordListParams{
		ZoneID: cloudflare.F(zoneId),
		Type:   cloudflare.F(dns.RecordListParamsTypeAAAA), // 只要ipv6
		Comment: cloudflare.F(
			dns.RecordListParamsComment{
				Startswith: cloudflare.F("DDNS-" + config.UniqueToken),
			},
		),
	})
	if err != nil {
		panic(err.Error())
	}

	// 批量队列，包含curd
	var batchPutParam []dns.BatchPutUnionParam
	var batchPostParam []dns.RecordBatchParamsPostUnion
	var batchDeleteParam []dns.RecordBatchParamsDelete

	var cannotDeleteList []string

	// 判断是否拥有dns
	for mac, configRecords := range config.RecordMap {

		// 配置文件中获取mac对应的record array
		for _, configRecord := range configRecords {

			// 检查cf中是否已都有macIpv6Map[mac]中的每一个ipv6
			var okIpv6IdList []int
			var isUpdated = false
			for _, ipv6 := range macIpv6Map[mac] {
				var sameRecordNameCount = 0
				ipv6IsOk := false
				for id, v := range cfRecordListResponse.Result {
					// 统计同名Record有多少个
					if v.Name == fmt.Sprintf("%s.%s", configRecord.Name, config.DomainName) {
						sameRecordNameCount++
					}

					// 注： 这里的configRecord.Name只是域名前缀，没有带主域名
					if v.Name == fmt.Sprintf("%s.%s", configRecord.Name, config.DomainName) && v.Content == ipv6 {
						okIpv6IdList = append(okIpv6IdList, id)
						ipv6IsOk = true
						break
					}
				}

				// 找到完全匹配的记录即可直接跳过
				if ipv6IsOk {
					continue
				}

				// 更新只要失败过一次不再进行for循环
				for id, v := range cfRecordListResponse.Result {
					// 不要修改对的cfRecord
					if v.Name == fmt.Sprintf("%s.%s", configRecord.Name, config.DomainName) && !slices.Contains(okIpv6IdList, id) {
						batchPutParam = append(batchPutParam, dns.BatchPutAAAARecordParam{
							ID: cloudflare.F(v.ID),
							AAAARecordParam: dns.AAAARecordParam{
								Name:    cloudflare.F(configRecord.Name),
								Type:    cloudflare.F(dns.AAAARecordTypeAAAA),
								Comment: cloudflare.F(fmt.Sprintf("DDNS-%s  %s", config.UniqueToken, configRecord.Comment)),
								Content: cloudflare.F(ipv6),
								Proxied: cloudflare.F(false), // 小黄云，代理后很慢
								// 氪金玩意，免费版用不了，额度为0,报错：DNS record has 1 tags, exceeding the quota of 0.
								//Tags: cloudflare.F([]dns.RecordTagsParam{
								//	"ddns",
								//}),
								TTL: cloudflare.F(dns.TTL(60)),
							},
						})
						// 加入修改队列后这个id也是对的了
						cannotDeleteList = append(cannotDeleteList, v.ID)
						okIpv6IdList = append(okIpv6IdList, id)
						isUpdated = true
						break
					}
				}

				// 更新失败，说明没有了，只能Posts新增了
				if !isUpdated {
					batchPostParam = append(batchPostParam, dns.RecordBatchParamsPost{
						Name:    cloudflare.F(configRecord.Name),
						Type:    cloudflare.F(dns.RecordBatchParamsPostsTypeAAAA),
						Comment: cloudflare.F(fmt.Sprintf("DDNS-%s  %s", config.UniqueToken, configRecord.Comment)),
						Content: cloudflare.F(ipv6),
						Proxied: cloudflare.F(false), // 小黄云，代理后很慢
						TTL:     cloudflare.F(dns.TTL(60)),
					})
				}

				// 若cfRecord的数量超过配置文件中的Record数量，则会删除无用的记录
				// 新增队列中为空时，才有可能出现该删除的情况
				if len(batchPostParam) > 0 {
					continue
				}

				if sameRecordNameCount != len(macIpv6Map[mac]) {
					// 此处删除仅考虑*已有但冗余*的情况
					for id, v := range cfRecordListResponse.Result {
						if v.Name == fmt.Sprintf("%s.%s", configRecord.Name, config.DomainName) && !slices.Contains(okIpv6IdList, id) {
							// 加入删除队列
							batchDeleteParam = append(batchDeleteParam, dns.RecordBatchParamsDelete{
								ID: cloudflare.F(v.ID),
							})
							continue
						}
					}
				}
			}
		}
	}

	// 当cloudflare dns中的带DDNS注释的记录域名前缀根本没有在当前配置文件中出现，需要及时清除避免堆积。
	for _, v := range cfRecordListResponse.Result {
		var recordIsExist = false
		for _, configRecords := range config.RecordMap {
			for _, existRecord := range configRecords {
				if v.Name == fmt.Sprintf("%s.%s", existRecord.Name, config.DomainName) {
					recordIsExist = true
				}
			}
		}
		if !recordIsExist {
			batchDeleteParam = append(batchDeleteParam, dns.RecordBatchParamsDelete{
				ID: cloudflare.F(v.ID),
			})
		}
	}

	// 当cf ipv6数大于有效邻居ipv6数应当删除无法访问的ipv6避免轮询到死ipv6
	for _, v := range cfRecordListResponse.Result {
		var recordIsExist = false
		for _, ipv6List := range macIpv6Map {
			for _, ipv6 := range ipv6List {
				if v.Content == ipv6 {
					recordIsExist = true
				}
			}
		}
		if !recordIsExist && !slices.Contains(cannotDeleteList, v.ID) {
			batchDeleteParam = append(batchDeleteParam, dns.RecordBatchParamsDelete{
				ID: cloudflare.F(v.ID),
			})
		}
	}

	if len(batchPutParam) == 0 && len(batchPostParam) == 0 && len(batchDeleteParam) == 0 {
		fmt.Println("无需进行任何操作，本次ddns已结束！")
		return
	}
	fmt.Printf("本次共需要更新%d条，新增%d条，删除%d条record\n",
		len(batchPutParam), len(batchPostParam), len(batchDeleteParam))

	body, err := json.Marshal(dns.RecordBatchParams{
		ZoneID:  cloudflare.F(zoneId),
		Deletes: cloudflare.F(batchDeleteParam),
		Puts:    cloudflare.F(batchPutParam),
		Posts:   cloudflare.F(batchPostParam),
	})
	if err != nil {
		fmt.Printf("Failed to marshal JSON: %v", err)
	}

	fmt.Println(string(body))

	// batch 批量操作执行顺序
	// Deletes -> Patches(部分覆盖) -> Puts(全量覆盖) -> Posts
	_, err = client.DNS.Records.Batch(context.TODO(), dns.RecordBatchParams{
		ZoneID:  cloudflare.F(zoneId),
		Deletes: cloudflare.F(batchDeleteParam),
		Puts:    cloudflare.F(batchPutParam),
		Posts:   cloudflare.F(batchPostParam),
	})
	if err != nil {
		panic(err.Error())
	}
	fmt.Println("ddns更新完毕！")
}

// 陈旧非REACHABLE ip强制检测是否存活
func ipv6NudTest(n netlink.Neigh) {
	if n.State != netlink.NUD_REACHABLE &&
		n.State != netlink.NUD_FAILED &&
		n.IP.To16() != nil &&
		n.IP.IsGlobalUnicast() && // 全局可达
		!n.IP.IsPrivate() && // 非 ULA（fd00::/8）
		!n.IP.IsLinkLocalUnicast() && // 非 fe80::/10
		!n.IP.IsLoopback() && // 非 ::1
		!n.IP.IsMulticast() && // 非 ff00::/8
		!n.IP.IsUnspecified() {
		// 设置 NUD_PROBE 来触发系统探测
		neigh := netlink.Neigh{
			LinkIndex: n.LinkIndex,
			IP:        n.IP,
			Family:    netlink.FAMILY_V6,
			State:     netlink.NUD_PROBE, // 关键点：强制系统发送 Neighbor Solicitation
		}

		// 设置邻居项（等价于 ip -6 neigh change ... nud probe）
		if err := netlink.NeighSet(&neigh); err != nil {
			fmt.Printf("\u001B[31m触发探测失败: %s，如果是权限不足，请用管理员身份运行。\u001B[0m \n", err.Error())
			return
		}
		fmt.Printf("\u001B[31m%s 状态陈旧，已发起邻居探测 (NUD_PROBE) \u001B[0m \n", n.IP.String())
	}
}

// 过滤掉非公网或陈旧ipv6
func isValidReachableGlobalIPv6(n netlink.Neigh) bool {
	return n.IP.To16() != nil &&
		n.IP.IsGlobalUnicast() && // 全局可达
		!n.IP.IsPrivate() && // 非 ULA（fd00::/8）
		!n.IP.IsLinkLocalUnicast() && // 非 fe80::/10
		!n.IP.IsLoopback() && // 非 ::1
		!n.IP.IsMulticast() && // 非 ff00::/8
		!n.IP.IsUnspecified() &&
		n.State == netlink.NUD_REACHABLE // 非 ::
}

// 获取本地配置文件, 与二进制应用程序同级
func loadConfig() (*Config, error) {
	exePath, err := os.Executable()
	if err != nil {
		return nil, err
	}
	dir := filepath.Dir(exePath)
	configPath := filepath.Join(dir, "config.json")

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	var cfg Config
	err = json.Unmarshal(data, &cfg)
	if err != nil {
		return nil, err
	}

	// 把所有 MAC 强制转成小写，避免程序遗漏
	lowered := make(map[string][]LocalRecord)
	for k, v := range cfg.RecordMap {
		lowered[strings.ToLower(k)] = v
	}
	cfg.RecordMap = lowered

	return &cfg, nil
}

// 通过三方工具获取本机ipv4(dmz可变ipv4情况)
func getLocalIpv4ByHttp() string {
	req, err := http.NewRequest("GET", config.LocalIpv4AddrApiUrl, nil)
	if err != nil {
		panic(err)
	}
	// 设置浏览器的 User-Agent，避免403
	req.Header.Set("User-Agent",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/120.0.0.0 Safari/537.36")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println(err.Error())
		return ""
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			fmt.Println(err.Error())
		}
	}(resp.Body)

	body, _ := io.ReadAll(resp.Body)
	ipv4 := strings.TrimSpace(string(body))
	return ipv4
}

// 通过三方工具获取本机ipv6(dmz可变ipv4情况)
func getLocalIpv6ByHttp() string {
	req, err := http.NewRequest("GET", config.LocalIpv6AddrApiUrl, nil)
	if err != nil {
		panic(err)
	}
	// 设置浏览器的 User-Agent，避免403
	req.Header.Set("User-Agent",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/120.0.0.0 Safari/537.36")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println(err.Error())
		return ""
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			fmt.Println(err.Error())
		}
	}(resp.Body)

	body, _ := io.ReadAll(resp.Body)
	ipv6 := strings.TrimSpace(string(body))
	return ipv6
}
