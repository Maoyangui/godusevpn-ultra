# Azure Windows x64 镜像：只读查询

2026-10-27 准备。以下在已经登录的 CloudShell Bash 运行，仅查询公开镜像 metadata；本机未调用 az、未读取 Azure 凭据，未创建 VM。实际返回前不声称某个 LTSC/旧 SKU 仍可获得。

依据：[Azure CLI vm image](https://learn.microsoft.com/en-us/cli/azure/vm/image?view=azure-cli-latest)；`--all` 从服务实时查询，`--architecture x64` 固定本批架构。`LOCATION` 应改为计划使用的实际区域，以下 eastus 仅示例。

```bash
LOCATION=eastus
az vm image list-offers --location "$LOCATION" --publisher MicrosoftWindowsDesktop --query '[].name' -o tsv
az vm image list-skus --location "$LOCATION" --publisher MicrosoftWindowsDesktop --offer windows-10 --query '[].name' -o tsv
az vm image list-skus --location "$LOCATION" --publisher MicrosoftWindowsDesktop --offer windows-11 --query '[].name' -o tsv
```

从返回中找真实匹配的 1809/rs5/LTSC2019、21H2/LTSC2021、22H2、23H2、24H2、25H2；名称里有同一个基础版本不证明是 LTSC edition。若想查看另一个实际返回的 offer，使用它的原名，不能凭空构造 SKU。

先可有界查询一个版本前缀，确认真实 SKU 后将 `SKU` 替换为完整返回值。`--sku` 在 list 中是部分匹配，因此必须用下一步 show 核对精确值。

```bash
az vm image list --all --architecture x64 --location "$LOCATION" --publisher MicrosoftWindowsDesktop --offer windows-10 --sku win10-22h2 --query '[].{urn:urn,sku:sku,version:version}' -o json
SKU='<exact SKU returned above>'
az vm image list --all --architecture x64 --location "$LOCATION" --publisher MicrosoftWindowsDesktop --offer windows-10 --sku "$SKU" --query '[].{urn:urn,sku:sku,version:version}' -o json
URN='<exact publisher:offer:sku:version returned above>'
az vm image show --location "$LOCATION" --urn "$URN" -o json
```

Windows 11 用实际 `windows-11` offer/SKU 做同样查询。最终保留 show 的完整公开 metadata，特别是 architecture、hyperVGeneration、imageDeprecationStatus、disallowed、features、精确版本；字段缺失/未知不当作支持。不要以 `latest` 作为最终可重现实测 pin，记录精确 URN，并在 VM 创建后另读 OS build/UBR/edition。

镜像显示与部署还受地区、退役和订阅可见性影响。微软 [Windows client dev/test](https://learn.microsoft.com/en-us/azure/virtual-machines/windows/client-images) 说明可用镜像及合适 Visual Studio/订阅权益；查到 SKU 不是部署授权或资源配额证明。若 LTSC 未出现，当前结论就是未确认可获得，不改用同 build 的 Server 冒充。

本文件没有 `vm create`、资源写入、条款接受、账户重置、网络设置或安装命令。实际创建测试资源另由主任务按用户授权与当前环境推进。

## 2026-10-27 实际只读查询

在现有测试订阅的 `eastasia` 区域实际查询到以下 x64 精确版本；它们只是可见镜像，尚未创建或启动这些新 VM，不代表功能验收通过。版本列表的 `--sku` 会作前缀匹配，Win11 `pro` 可能同时返回 `pron`；最终使用 `[?sku=='精确SKU']` 过滤，以免把 N 版当普通版。

| 目标 | 精确 SKU | 本次返回的镜像版本 |
|---|---|---|
| Win10 22H2 Pro | `win10-22h2-pro-g2` | `19045.6456.251117` |
| Win10 21H2 Enterprise LTSC | `win10-21h2-ent-ltsc-g2` | `19044.7807.261013` |
| Win10 1809 Enterprise；是否 LTSC 待实际 edition 核对 | `rs5-enterprise-g2` | `17763.9376.261010` |
| Win11 23H2 Enterprise | `win11-23h2-ent` | `22631.7652.261013` |
| Win11 24H2 Pro | `win11-24h2-pro` | `26100.9711.261011` |
| Win11 25H2 Pro | `win11-25h2-pro` | `26200.9711.261011` |

Win11 23H2 的 Pro SKU 本次未返回，Enterprise SKU 可见；不能因此声称 Pro 已实测。Win10 22H2 精确镜像详情返回 x64、Hyper-V V2、Windows、无额外 purchase plan；没有据此推断许可资格或安装后的 build/UBR。完整原始操作回执保留在忽略的私有 QA 目录，公开文档不包含账号、订阅或资源标识。
