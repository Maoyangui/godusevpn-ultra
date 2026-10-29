# 第二轮全量审计与修复台账

日期：2026-10-19。对象仅为 Ultra 副本。原项目不改；VPS、代码审计、模拟测试、真实保护效果分别记账。

## 审计结论

第一轮的21项问题仍按A编号保留；第二轮新增12项（C01/C02/C04/C05/C06及U01–U07），合计33项问题或需求缺口。C03补充A01的本机读取范围，不重复计数。优先级表示整改次序，不能将全部UI/功能缺口称为可利用漏洞。

本页为当前整改台账，原报告保留修复前快照。用户已明确本次 beta 只修复并验收现有功能；首次联网询问、独立浏览身份、通用凭据加密等独立工程保留路线图，不假称交付，也不把尚未开发的需求混成现有实现漏洞。**源码/fake/TEMP通过不等于真实UAC、SCM、WFP、MV3或浏览器效果通过；C04同账号主动攻击残余仍未关闭。**

完整证据和准确的触发前提见：

- [第一轮报告](AUDIT_REPORT.zh-CN.md)：R01–R14、N/S/B/D全部功能、A01–A21、扩展候选与57条实测用例。
- [核心独立复核](qa/AUDIT_CORE_RECHECK.zh-CN.md)：主IPC、服务存储、机器/用户边界、恢复与安装。
- [UI与检测独立复核](qa/AUDIT_UI_DETECTION_RECHECK.zh-CN.md)：检测信息、导出、扩展、简洁文案、问号及输入选择。
- [功能台账](FEATURE_STATUS.zh-CN.md)：未实现功能继续保留，不因审计或单项测试通过而自动完成。

## 新发现及本批处理

| 项目 | 结论 | 本批处理与验收限制 |
|---|---|---|
| C01 主IPC缺服务端认证 | P1，有本机抢占前提；不是网页远程抢占 | 请求前认证、持有句柄并发送前重核；GUI身份未知不修复/清闸。新兼容修复：安装实例严格绑定SCM当前PID/OwnProcess/规范LocalSystem配置与实际映像，普通UI不再强制读取SYSTEM令牌；仅明确前台分支保留提升令牌和代码pin。IPC/desktop回归及独立二审通过；新候选真实普通令牌GetState仍是发布前待验 |
| C02 核心服务数据owner/路径信任 | 条件成立时P1；须普通用户能预置或替换输入 | 源码、fake及独立二审通过：固定核心输入与未来子项继承写权限校验；可选隐私目录不参与VPN启动。目录pin补LIST_DIRECTORY，TEMP删除共享/正常创建与原子替换通过；真实默认ACL/并装迁移仍待测 |
| C03 诊断与日志面向本机Users可读 | A01范围补充 | 本批代码修复：已验证的logs/diag及全部历史后代收紧SY/BA，新增诊断不落机器共享ZIP。fake/TEMP DACL通过；真实跨账户新读取与普通GUI认证IPC仍待验，不能撤回此前复制/已打开内容 |
| C04 APPDATA可导致远程文件系统I/O | P2，路径条件成立时 | 部分修复、未关闭：拒绝远程/设备/不明盘及已有重解析路径；只读备份使用严格目录lease。写入为保留原子备份能力，仍不抵抗同账号并发原地重解析或主动盘符映射变化；无HTTP不等于整个进程零外联，没有观察到真实凭据外传 |
| C05 IPC不校验响应ID | P3协议一致性缺口 | 源码、GUI和独立mock通过；错误是“结果未知”，与身份未认证、服务确实缺席分开显示，不重试写入 |
| C06 机器journal加入具体账号SID | P2，明确账号范围 | 与A13一起代码/fake/TEMP通过；qa.2四项SYSTEM往返后独立读回，新机器store canonical empty、legacyAllowCount=0。旧含具名ACE对象按原字节/ACL显式归档保留，非自动迁移；qa.3及其他令牌/异常矩阵仍待验 |
| U01 无效Locale仍observed并导出 | P2 | 与A09一起修复并独立回核：本地NLS校验及规范名，空/无效保留unknown，不导出无效原串；仍不代表浏览器语言 |
| U02 CLI未说明机器journal未检查 | P2 | 序列化和独立回核通过：report/detection明确当前进程用户备份范围、machineJournalChecked=false/not_inspected；不据此推断机器健康 |
| U03 浏览器引导误开默认应用页 | P3 | 代码/DOM已改为隐私中心浏览器标签及扩展说明，不再调用默认应用页；没有声称自动连接用户实际浏览器配置 |
| U04 扩展7类控件帮助缺项 | P3 | 代码/DOM补齐标签、动态确认/取消、模式/来源、输入输出与launcher帮助；真实辅助技术及缩放仍待验 |
| U05 敏感链接帮助绝对承诺 | P3 | 已收窄为“识别到常见模式时保留，无法识别所有签名，复制前检查”；手动预览逻辑保留 |
| U06 权限允许/询问/拒绝选择未交付 | P2功能缺口 | 扩展正式contentSettings按类型提供选择/来源/确认；JS仅allow/block，摄像头/麦克风allow须明确HTTPS原点。fake/DOM通过；当前宿主新Edge配置仅通知example.invalid的block/ask/clear抽样实测，verified=false。其他API/企业覆盖/隐身待验，Windows每应用权限未因此实现 |
| U07 订阅绝对不离开本机承诺 | P2文案问题 | 中英文已说明本地保存、添加/更新会连接提供方；正常服务商请求不被说成未知第三方上传 |

代码修复记录独立保存于 [IPC认证](qa/IPC_AUTH_FIX.md)、[核心服务存储](qa/SERVICE_STORE_TRUST_FIX.md)、[机器恢复](qa/MACHINE_RECOVERY_FIX.md)、[地区与CLI范围](qa/LOCALE_AND_CLI_SCOPE_FIX.md)、[诊断最小化与私有存储](qa/DIAGNOSTIC_MINIMIZATION_FIX.md)、[前端/授权/扩展](qa/FRONTEND_AUDIT_FIX.md)、[维护与独立恢复](qa/MAINTENANCE_RECOVERY_FIX.md)。[IPC二审](qa/IPC_AUTH_SECOND_REVIEW.md)、[核心存储二审](qa/SERVICE_STORE_SECOND_REVIEW.md)、[维护二审](qa/MAINTENANCE_SECOND_REVIEW.md)保留开发中发现并关闭的分支问题，不能额外当成未关闭漏洞计数。单测/fake通过只证明所列边界，未自动取得安装、流量或真实浏览器验收。

## 第一轮 A01–A21 当前整改状态

“代码已修”表示当前实现和所列本地回归通过，不表示完整实机验收或整个问题范围无条件关闭。

| 项目 | 当前代码与本地证据 | 尚未取得的验收/保留边界 |
|---|---|---|
| A01 默认诊断分享风险 | 默认最小白名单摘要，不采集日志/配置/崩溃/网络命令；详情需逐项选入及敏感确认；缓存同一ZIP供预览/导出，GUI校验hash并新建私有文件。fake/合成TEMP及桌面契约通过 | 详情仍可能识别个人，不称匿名；真实GUI→服务→保存与跨账号读取待验。Web/Android仅保留最小摘要，详情/ZIP明确不支持，旧下载410 |
| A02 旧证据未失效 | 桌面/扩展失败、回焦和管理操作后标历史；桌面报告另有到期限制。fresh认证IPC摘要与分来源完成时间，缺来源不伪造。fake/DOM通过 | 扩展没有定时TTL；无全局监视外部软件；真实窗口/进程变化待验；读取时间不是运行时防泄漏实测 |
| A03 助手取消/失联不可重授权 | 显式状态与授权、single-flight、迟到结果失效、失败写不重试。协议/GUI fake通过 | 真实UAC同/替代账号、助手退出、父界面关闭待验 |
| A04 扩展并发丢恢复记录 | MV3共享队列及applying/restoring先记录，缺记录且可确认归属时显式清本扩展覆盖。mock通过；当前宿主新Edge配置真实五个同时请求全回执、中间active、最终恢复值及空journal | 真实worker终止/恢复、多窗口、多配置与企业控制权待验；本次不是崩溃恢复或流量验收 |
| A05 跨页旧确认写入 | 页面/操作世代检查、导航/草稿取消未提交确认、全局忙状态；DOM跨页不写通过 | 托盘/真实Wails生命周期待验；已提交动作不因导航假称回滚 |
| A06 未知过滤器误报移除 | 任一unknown对象使单条撤销返回unknown_remnants、verified=false及错误；专用fake覆盖未知键和读回失败 | 真实损坏元数据/持久WFP待验；未知对象保留，不猜测扩大删除 |
| A07 卸载维护竞态 | 同一受信Global mutex/显式lease覆盖HKLM、WFP、时区及官方cleanup→完整复核→固定服务卸载；fake/TEMP pin及二审通过 | 真多进程/abandoned/SCM卸载待验；只协调本产品入口，不拦外部管理员SCM或旧独立svc uninstall；Interactive可占锁造成Busy |
| A08 广播范围过宽 | 正文/帮助改为组播与有限广播，DOM范围核对通过 | 定向广播、单播发现、蓝牙和系统代发未覆盖；流量待验 |
| A09 地区读取失败缺项 | 与U01：有效NLS名称才observed，失败稳定unknown；本地/序列化回归通过 | 不代表浏览器语言，真实用户配置矩阵待验 |
| A10 DNS/日志/凭据缺finding | 独立安全摘要finding，模式/级别/天数白名单，缺状态unknown；DPAPI未实现明确说明；fake通过 | 不导出原DNS地址/秘密；配置不证明解析路径、旧日志删除或全部ACL受保护 |
| A11 预览与保存快照不同 | 后端快照/选择/预览token、默认排除环境和观察身份、保存同一字节；Go fake/TEMP与DOM通过 | 真实原生保存/权限组合待验；过期/期间失效拒绝不重采集 |
| A12 外联用途控制 | 新检测无在线探测、订阅文案已纠正；完整用途开关尚由独立任务处理 | 不把检测零外联等同整个客户端零外联；不能据文案关闭N06 |
| A13 SYSTEM机器恢复 | machineOnly/固定HKLM/机器journal分离；代码/fake/TEMP及qa.2四项真实SYSTEM apply/读回/restore通过，独立后验确认四值缺失、新store为空 | 仅当前Win11 Pro与四个初始缺失值；qa.3、旧值/外部冲突/崩溃/其他令牌矩阵待验，不扩至HKCU或用户时区 |
| A14 成功渲染成错误 | 中性结果与范围说明分开，重复路径/模式草稿禁提交；DOM通过 | 真实WFP安装/删除只按安装证据表达，不称流量已阻断 |
| A15 CLI权限冒充helper | direct_cli身份独立，helper只取明确桌面授权，序列化矩阵通过 | CLI当前用户journal范围仍明确机器未检查，身份不等于保护 |
| A16 hard permit笼统文案 | backend/模块README精准指定permitSelf的CLEAR_ACTION_RIGHT，不外推其他soft permit | 不改变guard；TUN/LAN/回环/其他产品真实仲裁待验 |
| A17 未知数量丢失 | network-firewall-unknown独立count；Rules空及无warning仍保留，fake通过 | 仅本次provider枚举，不称全系统完整清单 |
| A18 文件工具状态推断 | 明确本报告未包含文件检查，工具结果单独查看 | 未增加路径/图片元数据自动采集或混入导出 |
| A19 正文/帮助认知负担 | 卡片正文精简、范围入详情、帮助含目标名称、长状态折行；28张合成像素及DOM检查 | 真实读屏/键盘/多缩放与用户认知评估待验，不删必要能力边界 |
| A20 中断安装指引不足 | 新增中英人工恢复说明，脚本失败明确指向；Parser通过 | 首beta无自动续装/接管/删除；真实部分安装及恢复待验 |
| A21 GUI不可用缺时区CLI | timezone-report/restore只定位原令牌用户备份；最小bootstrap只建同步门禁、不碰机器store。fake/协议通过 | 真实权限/替代管理员UAC/时区恢复待验；共享系统设置，不代表浏览器同步 |

C04的[用户存储修订](qa/USER_STORE_LOCAL_PATH_FIX.md)和[独立二审](qa/LOCAL_STATE_SECOND_REVIEW.md)记录了分层：只读备份采用有效的数据写共享限制；写入保留原子替换能力，并保留主动并发改向的限制，不作已全面修复结论。严格只读与并发写入发生冲突时可拒绝本次备份访问/写入，不能因此断开VPN或撤掉既有guard。

Windows pin证据校正：仅申请READ_CONTROL/READ_ATTRIBUTES再拒绝DELETE共享不足以证明拒绝目录DELETE访问。C01/C02/A07已补FILE_LIST_DIRECTORY，并用TEMP目录实际验证持有时sharing violation、释放后可打开；C02正常创建/原子替换仍通过。C04只读同样以实际数据访问权拒writer；其写模式保留WRITE共享，因而同账号原地改reparse与主动DOS映射竞态仍存在，不能拿新DELETE pin证明它关闭。

完整回归还发现继承的`CheckNow`时序问题：Connected发布后才清空旧请求，会误吞新连接的立即健康检查。现将清旧队列与发布放入同一临界区，并对入队验证同锁；确定性复现及20轮定向回归通过，见[修复记录](qa/STATE_CHECKNOW_FIX.md)。该项独立于33项隐私审计台账，不涉及重写原目录或撤销guard。

## VPS实际进度

1. 原固定v1包传输、32条目/30个payload校验通过；没有用新构建覆盖旧证据。
2. SYSTEM令牌的两条只读命令通过：26项manual/not_checked，不能代表登录用户。
3. 已认证的交互管理员令牌两条只读命令通过：26项available/canApply、无读取失败或冲突；用户journal为空，机器journal范围另列未检查。
4. 用户逐项确认后，固定v1的`windows-clipboard-history`完成写前备份→apply→独立读回→restore。单独SYSTEM云命令再次确认原值不存在、目标journal记录不存在、机器managed条目0。
5. 上述v1结果仅证明该交互单项注册表往返，不能移用到新候选或功能运行行为。
6. qa.2完整候选文件重建/组装后，固定五条SYSTEM只读命令真实通过：三个SHA/静态版本、CLI/服务运行时版本、36项控制、45项finding及零规则/未知过滤器。machineJournalChecked=false，非登录用户或GUI验收。
7. 旧空store归档v1返回review_required，批次随即停止；独立读回源未动、目标缺失、四值缺失。TEMP复现父pin造成Win32 32，失败证据保留。单独审阅的v2释放本批父pin后只移动一次，重新root-first核目标字节/owner/group/DACL及源缺失，真实completed；未修ACL/删除/回滚，仍有受信稳定命名空间的释放窗口。
8. qa.2四个固定HKLM策略在SYSTEM下真实apply/读回/restore完成；独立后验确认四值缺失、新机器store两文件canonical empty/legacyAllowCount=0、保留的旧归档empty/legacyAllowCount=1。不是Windows功能、网络或所有用户策略实测。
9. qa.3因C01兼容修订重新构建，本地ZIP/352文件/351payload、PE/内嵌前端及WhatIf通过；固定文件传输、三EXE重建和352文件组装也在真实VPS私有目录完成，且明确未运行产品/安装器。qa.3真实产品只读命令、普通medium用户GetState、GUI/UAC/安装/流量仍待验，不继承qa.2结果。随后一次 qa.3 v1 preview 在真实机器上以 `SCM_unknown` fail-closed（`whatIfStarted=false`、未运行产品/安装器），不能记为安装预检通过；冻结 v1 不复用。IntPtr NULL-safe 的 v2 安装预检与 SCM readback 已完成本地脚本检查（79/11 与 22 项），尚未取得真实运行证据。

证据：[v1交互只读](vps/interactive-readonly-2026-10-19.json)、[v1单项往返](vps/policy-roundtrip-2026-10-19.json)、[qa.2只读](vps/beta-qa2-readonly-2026-10-19.json)、[归档失败](vps/beta-qa2-archive-attempt-2026-10-19.json)、[v2归档](vps/beta-qa2-archive-completed-2026-10-19.json)、[四项往返](vps/beta-qa2-policy-roundtrip-2026-10-19.json)、[独立后验](vps/beta-qa2-post-restore-readback-2026-10-19.json)。只保存固定计数、版本、哈希和作用域，不保存账号、地址、云资源标识、订阅、实际指纹或原始策略报告。

当前宿主另有[真实MV3有限验收](qa/MV3_EDGE_INTEGRATION.md)：新TEMP Edge配置内三项privacy设置及通知单origin样本往返、真实后台五请求串行化通过。不是VPS/原profile/企业策略/worker重启/所有站点或流量验收。页面四个本地资源请求不证明整个浏览器零外联。[原目录完整性](source-integrity.json)最近一次420文件比对及Git干净通过，只证明基线层。

Azure Bastion Developer 曾成功建立网页桌面并打开管理员命令提示符，但观察到临时用户配置文件，未取得真实令牌/UAC证明，也未确认产品或剪贴板命令执行；编辑 Clipboard 对话框后远程页失去响应。该会话不能作为 GUI/UAC、安装、恢复或断网测试证据，Developer 资源仍按独立会话记录保留，详见 [Bastion 会话记录](vps/bastion-developer-session-2026-10-19.json)。

## 修复顺序与逻辑门槛

1. 本批先完成C01/C02/C05/A13/C06及独立复核，明确未知结果、不重试写入；原隧道和guard不因可选隐私失败而撤销。
2. 本批已补诊断最小化/预览/本机私有存储、旧证据失效与同快照导出；CLI A15标签、助手重授权、旧页确认、扩展队列、未知过滤器撤销与维护窗口均有代码和专用回归。C04主动攻击残余继续部分修复，不靠文案关闭。
3. 当前beta先验收上述现有功能修复，不并入首次联网决策/独立身份/凭据迁移工程。真实机器journal只读检查不能由当前用户CLI report推断；N06用途控制完成前保持未交付标记。
4. 新包先只读验证，再按运行手册分批真实修改；v1单项结果不得冒充新包通过。旧v1 journal/lock可能含账号ACE，未经单独审查不得为通过新校验而自动重写/删除。
5. 有效快照仍保留并计费，1美元是预算而非Azure硬限额；磁盘快照还原尚未演练。单项或四项注册表恢复成功仍不足以宣称能安全做全部断网测试。

每一批必须记录预期、观察、偏差、恢复结果和剩余门槛。所有未实现和未实测项继续保留，当前仍是未签名开发预览。

## 工具执行偏差

前端修复期间一次未限定路径的Windows `python` launcher触发Python安装管理器更新至26.4，并自动安装Python3.15.0；该bootstrap发生网络请求，超出原定离线编辑/mock范围，已告知用户。它不是产品检测或产品默认外联，也不能据此声称测试全程无网络。其后未卸载、回滚或修改全局环境，改用既有Node/Go/apply_patch；详见[脱敏偏差记录](qa/TOOL_RUNTIME_DEVIATION.md)。本页不包含账号、个人路径、云标识或凭据。

10. qa.4 私有暂存与只读验收已完成：固定包远端哈希匹配；三 EXE 哈希/静态版本匹配；五条只读命令通过，SYSTEM 观察范围、36 控制、45 条 detection finding、0 firewall 规则；SCM v2 只读确认两个目标服务缺席且 CIM=0。GUI/UAC、普通 medium 用户、安装/卸载、策略恢复、浏览器/隧道/WFP/流量未由此批关闭。
11. qa.4 清理完成：临时隐藏传输 gist、本地分段和辅助脚本已删除；Azure 临时资源组等待后 `exists=false`。脱敏记录见 [qa4只读](vps/qa4-readonly-2026-10-20.json) 和 [qa4清理](vps/qa4-cleanup-2026-10-20.json)。

## 2026-10-21 修复后复核

本轮发现并修复两个会让预期安装预演误判的兼容问题：PowerShell `-WhatIf` 下 `Get-FileHash` 不返回对象，及首次加载 CIM 模块时产生额外 `Set-Alias` ShouldProcess 提示。前者改为 .NET SHA256 只读流，后者把只读 CIM/SCM 预检放在临时关闭 WhatIf 偏好的作用域中并在动作前恢复。两份安装器脚本内容一致，未执行安装、服务、产品或网络操作。

本地完整候选复核通过：ZIP/清单/PE/前端包审计、恶意 ZIP 夹具 44 项、安装预检 114 项、接收器合成测试、Go test/vet、公开发布扫描（`findings=[]`）均通过。当前候选 ZIP 为 37,933,559 字节，SHA256 `fed0866073a3f022800e8f70123af57dd6358e477944863c2c1a33b176dc8144`；清单 SHA256 `2b42f32810d8f27a0bcd7126f217946a41a5ee95b5f4b3c5c69453163f24ea48`。

最新临时 VM 传输使用 19 个分片；该段随后由下方实际安装批次更新，已取得远端合并、Expand、实际安装和服务/CLI 回执。普通 medium 用户/UAC、官方完整卸载成功、恢复冲突、浏览器完整矩阵、隧道/WFP/流量和发布门槛仍未关闭；旧 qa.4 远端哈希不能迁移到修复后包。

## 2026-10-21 实际安装批次补充

- 新建一次性 Windows Server 2022 Core VM 后先创建临时快照；修复后包在私有 stage 完成 19 段合并和 Expand，`files=407`、三项哈希匹配、私有目录检查通过。云资源名称和 stage ID 不写入公开树。
- 首次流程漏跑 Expand，安装器文件不存在；该次没有系统写入。补跑 Expand 后官方安装器返回 0，写入 `C:\Program Files\GodUseVPN Ultra` 并注册服务。
- SYSTEM 回读确认服务 Running/LocalSystem/OwnProcess、路径精确，CLI/service 版本 `0.01-beta-qa.4`，`privacy report` 返回 0；报告保留机器 journal 未检查状态。此证据不覆盖普通用户、UAC 或 GUI。
- 官方卸载脚本在机器清理阶段返回 1，按设计保留文件和恢复工具；随后匹配服务独立卸载成功（SCM=0、`not-installed`）。这不是官方完整卸载通过，原因与本批没有机器策略恢复对象有关。
- 资源组删除已提交，资源列表为空；删除轮询曾短暂为 `true`，最终 `az group exists` 返回 `false`。临时 gist 和本地分片已清理。
- 结论：实际安装、服务启动、CLI 只读和服务清理部分通过；普通用户/UAC、策略往返、浏览器完整矩阵、真实订阅隧道、WFP/流量仍待验收，因此不创建公开仓库、标签或 beta Release。

## 2026-10-21 候选静态与安装批次审计（尚非发布终审）

- Go 完整标签测试与 vet 通过；PowerShell hostile fixture 44、安装预检 114、接收器、qa.4 只读和 SCM v2 AST 测试通过。
- UI 重新核对：brand 6 profiles/正方形/居中、privacy 28 screenshots、time 25 screenshots、audit 20 screenshots，均无页面错误和外联；diagnostic/IPC identity DOM mock 通过。
- 当前包 `0.01-beta-qa.4` 的 406 payload/407 ZIP 成员、三项 amd64 PE、前端资源和 SHA256 通过，ZIP SHA256 为 `fed0866073a3f022800e8f70123af57dd6358e477944863c2c1a33b176dc8144`；公开发布扫描 `findings=[]`，其 source manifest 以最终命令输出为准。
- 原项目工作树仍干净；Ultra 无 diff 空白错误；临时安装目录、helper、分片和 gist 均已清理；敏感资源/凭据扫描为空。
- 全量审计没有把真实 SYSTEM 安装扩大为普通用户/UAC、官方完整卸载、浏览器全矩阵或真实订阅/WFP/流量通过。发布门槛仍未满足，因此不创建公开仓库、标签或 Release。
