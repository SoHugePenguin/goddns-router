<h1>DDNS-family-golang-app</h1>
<h3 style="color: red">目前仅支持ipv6，原理是利用mac 和 ip neigh网络邻居获取ipv6
<br/>所以应该在openWrt、iStoreOS-N100等路由器上linux环境运行。</h3>

<p style="color: darkorange">只是放在自己电脑或虚拟机上应该只能用本机ddns而不能家庭全部成员ddns，探测不积极</p>

linux部署方式(amd64)：
<p>CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o ddns-golang-app ./</p>


<p>config.json 格式示例：</p>
<h3>注意，mac，域名前后缀等参数以自己的为准，不要照抄</h3>
<pre>
{
  "uniqueToken": "iStoreOS-N100",
  "cloudflareEmail": "your-cloudflare-email",
  "cloudflareApiKey": "your-cloudflare-api-key",
  "domainName": "your-cloudflare-domain-name",
  "localIpv4AddrApiUrl": "https://api-ipv4.ip.sb/ip",
  "localIpv6AddrApiUrl": "https://api-ipv6.ip.sb/ip",
  "recordMap": {
    "00:00:00:00:00:00": [
      {
        "name": "n100",
        "comment": "路由器iStoreOS-N100 linux"
      }
    ],
    "10:ff:e0:06:7d:f5": [
      {
        "name": "mz72",
        "comment": "pve的远程运维web"
      }
    ],
    "a0:36:9f:f7:f7:d5": [
      {
        "name": "pve",
        "comment": "neigh"
      }
    ],
    "bc:24:11:40:47:48": [
      {
        "name": "virt210",
        "comment": "win2k22-Penguin-0"
      }
    ],
    "bc:24:11:42:15:81": [
      {
        "name": "virt211",
        "comment": "win2k22-XiaoYan-0"
      },
      {
        "name": "xiaoyan",
        "comment": "win2k22-XiaoYan-0 额外开的"
      }
    ]
  }
}
</pre>

<h3 style="color: red">如果你只是想给自己设备DDNS，recordMap只用填00:00:00:00:00:00就可以了</h3>
<pre>
{
  "uniqueToken": "my-computer001",
  "cloudflareEmail": "your-cloudflare-email",
  "cloudflareApiKey": "your-cloudflare-api-key",
  "domainName": "your-cloudflare-domain-name",
  "localIpv4AddrApiUrl": "https://api-ipv4.ip.sb/ip",
  "localIpv6AddrApiUrl": "https://api-ipv6.ip.sb/ip",
  "recordMap": {
    "00:00:00:00:00:00": [
      {
        "name": "your-name",
        "comment": "我的电脑"
      },
      {
        "name": "your-name222",
        "comment": "我的电脑别名"
      }
    ]
  }
}
</pre>