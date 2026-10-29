# VPS 三文件增量工具：本地验证

2026-10-19。工具为独立QA文件传输辅助，不修改产品实现，不自动上传、安装或执行EXE。

## 固定格式与调用

```text
node Build-VpsDelta.cjs --base-dir <old-three-exe-directory> --new-dir <new-three-exe-directory> --out-dir <new-delta-package-directory>
node Build-VpsDelta.cjs --self-test
powershell.exe -NoProfile -NonInteractive -File Apply-VpsDelta.ps1 -BaseDirectory <old-three-exe-directory> -DeltaDirectory <uploaded-delta-package-directory> -OutputDirectory <existing-private-QA-parent>\delta-<fresh-32-lowercase-hex> -ExpectedManifestSha256 <independently-approved-64-hex>
```

只接受`godusevpn.exe`、`godusevpn-cli.exe`、`godusevpn-svc.exe`。每个gzip指令流包含固定magic、新长度、COPY旧偏移/长度、LITERAL长度/字节和结束标记。固定manifest记录每个base/new SHA256与长度及patch SHA256/压缩和解压长度。每文件最多128MiB，三个结果总计最多384MiB，指令数、解压大小、JSON大小及所有读写边界有上限。

Node滚动弱校验只用于候选定位，复制前精确比较字节；生成后由内存decoder重建复核。PowerShell5.1先核独立manifest哈希，再严格解析固定schema、唯一三文件、整数长度，核base/patch哈希，内存重建及new哈希。**三个结果全部通过后**才原子新建`delta-<id>`目录，逐文件CREATE_NEW写入。失败不覆盖、接管、自动重试或删除现有对象；I/O失败后保留partial目录，人工核对并另选新ID。

输入/输出父级须明确为本地固定磁盘、无reparse的既有目录。卷根参数提前拒绝，避免PS5.1将`C:\`去尾后变为驱动器相对`C:`。工具不核对既有目录私有ACL，也不修改它们；远端调用者须独立确认私有QA父级并在使用结果前重读落盘EXE哈希。它不抵抗可主动改变同账户/管理员命名空间的进程。

builder生成无optional header的单个gzip member。PS5.1原生GZipStream能接受某些缺失trailer或尾部垃圾；独立审阅后补固定基础header、footer ISIZE、解压后显式CRC32，CRC以固定C#内存表实现，不用PowerShell逐字节循环。它不是通用deflate/多member容器认证器，不能据此保证识别所有精心构造的member边界；独立批准的compressed SHA和最终EXE SHA才是内容信任锚。该发现不属于产品功能漏洞，也没有绕过批准manifest/new哈希。

## 已执行验证

- Node语法、5种算法往返、5种坏指令流拒绝通过。
- 真实Windows PowerShell5.1进程的17个合成TEMP用例通过：正常重建；已有输出拒绝且sentinel不变；重复条目；超大长度；blockBytes错误类型；base哈希错；解压长度不符；patch损坏；COPY越界；LITERAL截断；target哈希错；缺base；缺1字节trailer；缺整个trailer；追加垃圾；CRC错；ISIZE错。
- PowerShell Parser通过。所有失败用例均未创建结果目录，成功仅三个固定文件；合成文件按跟踪清单逐叶清理，没有递归删除或未知路径清理。
- 最终decoder在本地对真实旧v1→qa.2的三EXE仅做数据重建，独立重读输出哈希全部匹配新包。没有执行这些EXE，没有改旧dist，没有Azure/网络、服务、注册表、WFP、UAC或产品操作。

旧目录版本为`0.1.0-ultra.1`，新目录版本为`0.01-beta-qa.2`。对应patch保持不变：

| 项目 | 数量/长度 |
| --- | ---: |
| 固定EXE | 3 |
| base合计 | 67,457,024 bytes |
| new合计 | 67,999,232 bytes |
| 三gzip patch合计 | 16,029,392 bytes（约15.29MiB） |

manifest SHA256：`8f81b6a3a78d500b8d1423ecd1d951beeb81a697e2b50c5190aba8f480835be5`。

最终`Apply-VpsDelta.ps1` SHA256：`e4035a5be9d064f3ad51ef79ecce87dc616edd318b87ad32d67fc6862a4a6adf`。

工具输出的哈希/计数只证明上述文件重建一致，不代表新包的CLI、GUI、安装、权限或流量已经验收。base ZIP解包、Azure分块传输、落盘ACL复核及产品命令执行不由这两个工具负责。无真实目标、账号、地址、云ID、订阅或凭据参数被写入本记录。
