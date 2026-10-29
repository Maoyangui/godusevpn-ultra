# Ultra QA：界面模拟与受控 Windows CLI 检查

这些 Node.js 脚本使用原始 `web/dist/index.html` 和固定的 Wails mock，在独立临时浏览器配置中运行。没有真实后端、管理员助手、UAC、系统设置、文件选择器或用户文件读取。图片处理与导出只返回虚构结果，不调用真实文件工具。

页面请求通过 Playwright 路由直接从仓库静态资源返回；只允许本地测试来源 `http://127.0.0.1:48731`，其余页面请求全部终止。不启动监听端口，不访问在线检测站点，不上传数据。浏览器以 headless 模式使用新临时配置启动，关闭常见后台联网和同步；脚本不读取用户现有浏览器配置。

## 依赖与执行

需要 Node.js 18 或更新版本、已经可用的 Playwright 模块，以及已安装的 Edge 或兼容 Chromium 可执行文件。脚本不会安装依赖或下载浏览器，也不修改项目依赖文件。所有环境路径通过命令行传入，源码没有固定用户目录。

在项目根目录执行，使用真实路径替换占位符；路径含空格时保留引号：

```powershell
node docs/ultra/qa/audit-qa.cjs "<Playwright 模块的绝对路径>" "<Edge 或 Chromium 可执行文件路径>"
```

第三个参数可指定截图输出目录；省略时写入 `docs/ultra/screenshots`。使用 `--help` 查看参数格式。脚本也可从其他工作目录运行，仓库根目录按脚本自身位置解析。

## 选择需要的检查

| 脚本 | 范围 | 输出记录 |
|---|---|---|
| `pixel-qa.cjs` | 初始整套隐私、检测、数据、网络、确认框和轻提示；420×760 与 900×900，深浅主题 | `qa-results.json` |
| `time-qa.cjs` | 仅时区卡、系统页及其确认、失败、读回、恢复、无列表降级 | `time-qa-results.json` |
| `audit-qa.cjs` | 最终修正涉及的无订阅入口、只读证据、说明卡定位、未知状态、自测问号及遗留过滤器提醒 | `audit-qa-results.json` |
| `brand-square-qa.cjs` | 主Logo方形、主体居中、加粗草写转曲边界、深浅主题与抽屉保持原样 | `ultra-brand-square-results.json` |

按修改范围选择脚本，避免没有新变化时重复生成整套截图。上表脚本均检查页面异常和外部页面请求，断言失败时返回非零退出码。它们不会把编译、配置回读或过滤器存在当作真实隐私保护验收。

品牌脚本使用前两个路径参数，检查320/420宽与1×/2×的六种组合。`ipc-identity-ui.cjs`及仓库根的`browser-extension/ui-tests.cjs`另用jsdom模块路径为首个参数；它们仅执行隔离DOM mock，不操作服务或系统。

`diagnostic-fixture-check.ps1`用Go overlay执行现有诊断设置副本回归：系统诊断替换为虚构文本，后续订阅请求替换为拒绝联网的transport。产品文件保持原样；通过不表示真实诊断已消除设备信息。脚本生成的临时测试文件不包含实际诊断数据。

发行包保留审计记录、最小化VPS结果及演示截图，使文档相对链接可查看。下面的VPS脚本固定于v1及其哈希，是历史受控测试工具；新包不得直接套用或绕过其版本校验。复制这些文档不会自动执行其中脚本。

`pixel-qa.cjs` 和 `time-qa.cjs` 保存了先前实际使用的完整 mock 测试流程。`audit-qa.cjs` 是最后修改后重新执行的局部回归。所有截图内容均为演示数据，不代表开发机器或用户设备的真实隐私状态。浏览器自身的截图、字体和原生选择菜单可能因版本与系统不同而有细小差别。

## Windows 实测前的只读检查

`Inspect-TestEnvironment.ps1` 是独立的 Windows PowerShell 5.1 脚本，推荐64位宿主。它不属于上面的界面 mock，也不会执行安装、提权、网络探测或设置修改。

```powershell
powershell.exe -NoProfile -NonInteractive -File .\docs\ultra\qa\Inspect-TestEnvironment.ps1
```

输出仅包含系统版别/build/架构、当前令牌类型、固定服务和浏览器运行时版本、默认数据/备份是否存在。不输出用户名、SID、主机名、路径、IP/MAC、凭据或实际备份内容。缺失与读取失败分别表示；备份存在不等于可恢复。便携浏览器、自定义数据位置与固定版本 WebView2 不在检查范围内。32位宿主在64位系统上未发现浏览器时保留 `unknown`。通过云命令执行时，用户范围仅代表该次令牌。

PowerShell 语法与纯函数检查不代替目标机验证；具体执行结果应另行记录。后续验收范围见[Windows VPS 实测步骤](../VPS_TEST_RUNBOOK.zh-CN.md)。

## 经 Azure Run Command 暂存固定包与有限 CLI 检查

下面是三个独立、必须依次核对结果的步骤：`Stage-VpsPackage.py` → `Expand-VpsCliSmoke.ps1` → `Vps-ReadOnlySmoke.ps1`。它们不安装产品，不导入订阅，不启动 GUI、服务或管理员助手，不修改策略、时区、WFP、网络或远程管理设置。第三步会执行两个固定的 CLI 只读命令，不能称为“整个流程没有执行产品”。

流程只针对下表这一份已审阅的预览包。哈希从可信的本地构建/审阅记录取得，不用下载包内自带的校验文件单独证明发布者身份；哈希匹配也不等于代码签名或安全保证。重新构建后必须重新核对包和脚本，不能临时放宽校验。

| 对象 | 固定 SHA-256 |
|---|---|
| 完整 ZIP | `a8717578892160bf45576882cf242fce48ccb8759726878ff3c6a00f08cda746` |
| ZIP 中的 SHA256SUMS | `16c41ad2296575cdf0cdcd46f3abfa6227cfc4601ea7acd842cd94b669cd0ad6` |
| godusevpn-cli.exe | `bf728ab1a000b4a8aae5329d7ba76cc25aae4c7a87667fecc0bfdf4d9f5ffaa1` |

### 前提与显式目标

- 使用已经授权、可恢复的专用测试 VM。先核对 VM、执行身份、可信本地 ProgramData 及其祖先、QA 路径所有者/写权限和非重解析条件。不能把脚本的局部检查当作整个系统已经可信。
- 使用已登录的 Bash Cloud Shell、Python 3.10+ 和已有 Azure CLI。脚本不自动登录、安装依赖或切换默认订阅。可使用不关联存储账户的临时会话；会话关闭后本地上传文件可能丢失。
- 通过用户明确的文件上传，将这三个审阅过的 QA 脚本和固定 ZIP 放入同一 Cloud Shell 工作目录。不要上传密码、真实订阅内容或私人文件；不需要公网 SSH、公开下载链接或第三方文件托管。
- Action Run Command 在 Windows 中以 SYSTEM 执行，同一 VM 一次只运行一个命令；本地超时不能取消已经提交的远端操作。先等待前一操作完成，再执行下一步。
- 下列值全部是占位符，必须由操作者明确替换。不要把实际资源标识或认证资料写入本 README、脚本默认值、Git 或共享测试报告。

在 Cloud Shell Bash 中设置本次目标；`ULTRA_QA_STAGE` 使用新生成的、小写32位 UUID4 hex，续传时保留同一值：

```bash
ULTRA_QA_SUBSCRIPTION='<subscription-guid>'
ULTRA_QA_GROUP='<resource-group>'
ULTRA_QA_VM='<vm-resource-name>'
ULTRA_QA_STAGE='<fresh-lowercase-32-hex-uuid4>'
ULTRA_QA_ZIP='<Cloud-Shell-local-ZIP-path>'
```

仅需生成新 stage ID 时，可在本地执行 `python3 -c 'import uuid; print(uuid.uuid4().hex)'`，再明确填入变量；它不是 Azure target ID。后续命令始终传入同一组目标参数，不依赖 Azure 默认资源组或订阅。

### 第一步：分块暂存并校验 ZIP

先做不调用 Azure 的本地检查：

```bash
python3 Stage-VpsPackage.py --self-test
python3 Stage-VpsPackage.py --dry-run --zip "$ULTRA_QA_ZIP" \
  --subscription "$ULTRA_QA_SUBSCRIPTION" --resource-group "$ULTRA_QA_GROUP" \
  --vm-name "$ULTRA_QA_VM" --stage-id "$ULTRA_QA_STAGE"
```

确认本地 ZIP 哈希匹配、所有请求在限制内后，再明确执行传输：

```bash
python3 Stage-VpsPackage.py --zip "$ULTRA_QA_ZIP" \
  --subscription "$ULTRA_QA_SUBSCRIPTION" --resource-group "$ULTRA_QA_GROUP" \
  --vm-name "$ULTRA_QA_VM" --stage-id "$ULTRA_QA_STAGE"
```

固定包有13块，每块原始数据不超过2 MiB，生成的 Run Command JSON 小于4,000,000字节；调用串行执行。远端只写 KnownFolder ProgramData 下的 `GodUseVPN-Ultra-QA/<stage-id>`，不提前创建真实产品的数据目录。该阶段保留 `stage.json`、分块、合并后的 `preview.zip`，失败时也可能保留唯一命名的合并临时文件；**“只落盘三个文件”指下一步有限解压，不是整个暂存阶段的文件数量。**

成功必须得到最终 `PACKAGE ... sha256=match`。每个 part 用 CreateNew 写入；同 stage 已有文件只在大小与 SHA 均相符时复用，不覆盖、不接管 ACL，也不删除冲突或不完整对象。错误立即停止，没有自动重试。请求超时、Cloud Shell 中断或结果未知时，先确认上一个 Run Command 已结束，再以相同 ZIP、显式 target 和 stage ID 重跑；冲突对象保留待审阅，必要时使用新的 stage ID。不要为了续传执行递归清理。

### 第二步：只解出 CLI 与两份校验资料

仅在第一步完成后执行：

```bash
az vm run-command invoke --subscription "$ULTRA_QA_SUBSCRIPTION" \
  --resource-group "$ULTRA_QA_GROUP" --name "$ULTRA_QA_VM" \
  --command-id RunPowerShellScript --scripts @Expand-VpsCliSmoke.ps1 \
  --parameters "StageID=$ULTRA_QA_STAGE" --only-show-errors --output json
```

脚本要求64位 Windows PowerShell 5.1+ 和 SYSTEM。它重新核验固定 ZIP、条目路径/类型/数量、固定校验清单及所有 payload 的 SHA，在该 stage 的**新建私有** `cli-smoke` 子目录仅写入：

1. `godusevpn-cli.exe`
2. `SHA256SUMS`
3. `ULTRA_PACKAGE.json`

不会解出 GUI、服务程序、安装器或浏览器扩展，也不会执行任何产品。成功应同时满足 `status=completed`、`smokeReady=true`、`extractedFileCount=3`、ZIP/CLI 哈希匹配。`smokeReady` 只表示准备好下一项有限 CLI 检查，不是已安装或通过保护验收。

输出目录已存在、包不符或任何步骤失败时停止；不覆盖、删除、修复或自动续解已有 `cli-smoke`。失败残留必须先审阅，不能反复盲跑；传输可续传不等于解压也可续传。

### 第三步：SYSTEM 范围的 CLI 只读 smoke

从目标 VM 已确认的 KnownFolder ProgramData 路径构成**同一 stage 的 `cli-smoke` 绝对路径**，明确填入下一变量；不要默认 ProgramData 必在某个盘符。该路径仅为本次参数，不写入共享报告。

```bash
ULTRA_QA_PACKAGE_DIR='<Windows-absolute-path-to-this-stage-cli-smoke>'
```

实际路径是 `ProgramData\GodUseVPN-Ultra-QA\<same-stage-id>\cli-smoke` 对应的 Windows 本地绝对路径，使用反斜杠；不要将占位符原样执行。确认暂存路径/祖先仍可信后，再运行：

```bash
az vm run-command invoke --subscription "$ULTRA_QA_SUBSCRIPTION" \
  --resource-group "$ULTRA_QA_GROUP" --name "$ULTRA_QA_VM" \
  --command-id RunPowerShellScript --scripts @Vps-ReadOnlySmoke.ps1 \
  --parameters "PackageDirectory=$ULTRA_QA_PACKAGE_DIR" \
    'ExpectedCliSha256=bf728ab1a000b4a8aae5329d7ba76cc25aae4c7a87667fecc0bfdf4d9f5ffaa1' \
  --only-show-errors --output json
```

脚本在再次核验固定 CLI 哈希并持有只读句柄后，只运行 `version` 和 `privacy report`。输出是经过固定字段与枚举聚合的状态/计数，不输出原始 CLI stdout/stderr、策略文本、真实路径或机器标识。CLI 超时只结束本次固定 CLI 子进程，不按文件名结束其他进程。Azure 外层错误仍可能包含资源标识；不要把原始 Azure 错误、执行脚本或控制台记录直接复制到共享报告，只记录最少状态与已脱敏计数。

**SYSTEM 下的 HKCU、APPDATA、用户 journal 和 BackupHealthy 只代表该次 SYSTEM 令牌。** 它们不是 RDP/GUI 用户的浏览器策略或备份，也不证明机器 journal 已检查或能恢复。`canApply`、策略值匹配、版本命令成功与 `status=completed` 都不是实际访问受限或匿名性证明。

### 该流程不构成哪些验收

- 不证明普通用户 GUI 能启动、WebView2可用、同账号/替代账号UAC正确、管理员助手可重连，或用户浏览器接受了策略。
- 不做 `privacy detection`、service version、策略应用/恢复、`cleanup-machine`、WFP写入、安装卸载、隧道连接、订阅导入、测速或实际网络路径测试。
- 不建立独立恢复通道，不验证快照/控制台恢复，也不能用SYSTEM只读报告证明已有机器策略可恢复。SYSTEM恢复兼容性须另行验收；没有managed策略时清理成功也不能证明有策略时能恢复。
- 不代表整个Windows系统没有写入：VM agent及Windows可能记录Run Command、进程、安全或杀毒日志；第三步“只读”仅描述本脚本与已审阅的两条应用命令路径，不是零系统痕迹承诺。
- 不扩大后续测试授权。真实GUI、UAC、机器策略、持久WFP与断网恢复，按[实测步骤与恢复前提](../VPS_TEST_RUNBOOK.zh-CN.md)分批进行；当前脚本成功不能替代那些门槛。

### 固定用途的 Cloud Shell 调度脚本

`Run-VpsReadOnlyQa.py` 可分别调度上述第二、三步，避免手写多层引号。把它与两份原样 PowerShell 脚本放在同一 Cloud Shell 目录，先核对来自本地审查版本的 SHA-256，再使用已核实的显式目标变量：

```bash
python3 Run-VpsReadOnlyQa.py --subscription "$ULTRA_QA_SUBSCRIPTION" \
  --resource-group "$ULTRA_QA_GROUP" --vm-name "$ULTRA_QA_VM" \
  --stage-id "$ULTRA_QA_STAGE" --step expand
# 观察 expand 完成且 smokeReady=true，再单独调度已获授权的只读命令：
python3 Run-VpsReadOnlyQa.py --subscription "$ULTRA_QA_SUBSCRIPTION" \
  --resource-group "$ULTRA_QA_GROUP" --vm-name "$ULTRA_QA_VM" \
  --stage-id "$ULTRA_QA_STAGE" --step smoke
```

两份脚本在 Cloud Shell 与 Windows 客体内均核对固定源哈希；启动标签只绑定本次调用，不能当作完成回执。只有唯一、完整、字段受限的结果被接受。不自动重试、不回显原始 Azure 响应；超时或结果不明时，先确认前一次操作已结束及远端状态。`expand` 创建新输出且拒绝覆盖，不能盲目重复运行。该工具只适用于脚本明确固定的 v1 预览 ZIP/CLI，不对其他新包自动放宽哈希或白名单。

## 与其他验证的边界

- 本目录的 mock 界面脚本验证真实浏览器中的像素布局、滚动、键盘、轻提示、筛选及失败隔离；它们不验证真实注册表、WFP 仲裁、UAC 或时区修改。
- 独立 DOM 集成检查位于 `browser-extension/ui-tests.cjs`，覆盖新增控件旁问号、manual 控制项导航、未知状态、原始数据不落前端日志等。
- 浏览器扩展的 `chrome.privacy` / `contentSettings` mock 回归位于 `browser-extension/tests.cjs`；这些脚本不安装扩展或修改真实浏览器策略。
- 真实浏览器策略接纳、站点例外、原生应用范围、网络路径和重启恢复仍需单独实机验收。
