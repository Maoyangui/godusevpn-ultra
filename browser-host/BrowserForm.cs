using System;
using System.Collections;
using System.Collections.Generic;
using System.Drawing;
using System.IO;
using System.Reflection;
using System.Text;
using System.Threading;
using System.Threading.Tasks;
using System.Net.Sockets;
using System.Windows.Forms;
using Microsoft.Web.WebView2.Core;
using Microsoft.Web.WebView2.WinForms;

namespace GodUseVPN.Ultra.BrowserHost
{
    internal sealed class BrowserForm : Form
    {
        private const string Bootstrap = "https://ultra-browser-bootstrap.invalid/";
        private const string BootstrapFrame = "https://ultra-browser-bootstrap.invalid/frame";
        private const string BootstrapIdentity = "https://ultra-browser-bootstrap.invalid/identity-probe";
        private readonly LaunchOptions options;
        private readonly WindowsGuard binding;
        private readonly string runtime;
        private readonly PipeLease lease;
        private readonly NavigationGate gate = new NavigationGate();
        private readonly TextBox address = new TextBox { Dock = DockStyle.Fill, Enabled = false };
        private readonly Button go = new Button { Text = "访问", Dock = DockStyle.Right, Width = 64, Enabled = false };
        private readonly Button back = new Button { Text = "后退", Dock = DockStyle.Right, Width = 56, Enabled = false };
        private readonly Button forward = new Button { Text = "前进", Dock = DockStyle.Right, Width = 56, Enabled = false };
        private readonly Button reload = new Button { Text = "刷新", Dock = DockStyle.Right, Width = 56, Enabled = false };
        private readonly Label state = new Label { Dock = DockStyle.Bottom, Height = 28, Text = "正在核验连接与浏览器配置…", AutoEllipsis = true };
        private readonly Label summary = new Label { Dock = DockStyle.Bottom, Height = 48, AutoEllipsis = true };
        private readonly WebView2 view = new WebView2 { Dock = DockStyle.Fill, AllowExternalDrop = false, Visible = false };
        private readonly System.Windows.Forms.Timer watcher = new System.Windows.Forms.Timer { Interval = 250 };
        private SocksRelay relay;
        private CoreWebView2Environment environment;
        private WindowsGuard.DataLease dataLease;
        private IDictionary<string, object> nativeHints;
        private string nativeUserAgent;
        private string nativeModel, nativePlatform;
        private RuntimeIdentity identity;
        private double displayScale = 1;
        private volatile bool revoked;
        private bool bootstrap;
        private bool identityProbe;
        private TaskCompletionSource<bool> bootstrapComplete;
        private CoreWebView2DevToolsProtocolEventReceiver targets;
        internal bool WasReady { get; private set; }

        internal BrowserForm(LaunchOptions config, WindowsGuard guard, string runtimeDirectory, PipeLease pipeLease)
        {
            options = config; binding = guard; runtime = runtimeDirectory; lease = pipeLease;
            var version = (AssemblyInformationalVersionAttribute)Attribute.GetCustomAttribute(typeof(BrowserForm).Assembly, typeof(AssemblyInformationalVersionAttribute));
            Text = "GodUseVPN Ultra " + (version == null ? "" : version.InformationalVersion) + " — 独立浏览器";
            AutoScaleMode = AutoScaleMode.None;
            const int chromeHeight = 148;
            int contentWidth = config.Width, contentHeight = config.Height;
            if (config.Environment.Mobile)
            {
                Size available = Screen.PrimaryScreen.WorkingArea.Size;
                displayScale = RuntimeIdentity.FitMobileScale(config.Width, config.Height, available.Width, available.Height);
                contentWidth = Math.Max(320, (int)Math.Ceiling(config.Width * displayScale)); contentHeight = Math.Max(240, (int)Math.Ceiling(config.Height * displayScale));
            }
            ClientSize = new Size(contentWidth, contentHeight + chromeHeight); MinimumSize = SizeFromClientSize(new Size(config.Environment.Mobile ? 320 : 800, (config.Environment.Mobile ? 240 : 600) + chromeHeight));
            var toolbar = new TableLayoutPanel { Dock = DockStyle.Top, Height = 72, Padding = new Padding(6), ColumnCount = 1, RowCount = 2 };
            toolbar.RowStyles.Add(new RowStyle(SizeType.Absolute, 28)); toolbar.RowStyles.Add(new RowStyle(SizeType.Absolute, 28));
            var buttons = new FlowLayoutPanel { Dock = DockStyle.Fill, WrapContents = false, FlowDirection = FlowDirection.LeftToRight };
            foreach (Button button in new[] { back, forward, reload, go }) { button.Dock = DockStyle.None; button.Height = 24; button.Margin = new Padding(0, 1, 6, 0); buttons.Controls.Add(button); }
            toolbar.Controls.Add(address, 0, 0); toolbar.Controls.Add(buttons, 0, 1);
            summary.Text = "语言：" + config.Locale + " ｜ 时区：" + config.Timezone + " ｜ 分辨率：" + config.Width + " × " + config.Height + "\n" +
                (config.Graphics.Metadata ? "WebGL 厂商：" + config.Graphics.Vendor + " ｜ 设备：" + config.Graphics.Renderer + "（仅 JS 名称，不改变硬件或渲染像素，可检测）" : "GPU 名称：保持原生");
            Controls.Add(view); Controls.Add(toolbar); Controls.Add(summary); Controls.Add(state);
            go.Click += delegate { NavigateAddress(); };
            back.Click += delegate { BrowserCommand("back"); };
            forward.Click += delegate { BrowserCommand("forward"); };
            reload.Click += delegate { BrowserCommand("reload"); };
            address.KeyDown += delegate(object sender, KeyEventArgs e) { if (e.KeyCode == Keys.Enter) { e.SuppressKeyPress = true; NavigateAddress(); } };
            watcher.Tick += delegate { try { if (!lease.Live) throw LaunchOptions.Reject("lease_expired"); binding.CheckAlive(); } catch { Revoke(); } };
            Shown += async delegate { await Initialize(); };
            FormClosing += delegate { Revoke(); watcher.Stop(); };
        }

        internal void Revoke()
        {
            revoked = true; gate.Close();
            var current = relay; if (current != null) current.Dispose();
            if (IsHandleCreated && !IsDisposed)
            {
                try { BeginInvoke((Action)delegate { address.Enabled = go.Enabled = back.Enabled = forward.Enabled = reload.Enabled = false; state.Text = "会话已关闭：连接或配置核验失效"; view.Dispose(); Close(); }); }
                catch (InvalidOperationException) { }
            }
        }

        private async Task Initialize()
        {
            try
            {
                if (revoked) { Close(); return; }
                watcher.Start(); binding.CheckAlive();
                dataLease = WindowsGuard.NewDataDirectory(options.ProfileId);
                string data = dataLease.Path;
                relay = new SocksRelay(binding, options, AuthorizeFrontend, Revoke);
                var environmentOptions = new CoreWebView2EnvironmentOptions
                {
                    AdditionalBrowserArguments = LaunchOptions.BrowserArguments(relay.Port, options.WebGpuDisabled), Language = options.Language,
                    ExclusiveUserDataFolderAccess = true, AllowSingleSignOnUsingOSPrimaryAccount = false,
                    AreBrowserExtensionsEnabled = false, IsCustomCrashReportingEnabled = true,
                    ReleaseChannels = CoreWebView2ReleaseChannels.Stable
                };
                environment = await Deadline(CoreWebView2Environment.CreateAsync(runtime, data, environmentOptions));
                if (revoked) return;
                if (!String.Equals(environment.UserDataFolder, data, StringComparison.OrdinalIgnoreCase) || environment.BrowserVersionString != Path.GetFileName(runtime)) throw LaunchOptions.Reject("environment_readback_mismatch");
                var controller = environment.CreateCoreWebView2ControllerOptions();
                controller.ProfileName = options.ProfileId; controller.IsInPrivateModeEnabled = true;
                await Deadline(view.EnsureCoreWebView2Async(environment, controller));
                if (revoked) return;
                CoreWebView2 core = view.CoreWebView2;
                WindowsGuard.CheckBrowserPid(core.BrowserProcessId, runtime);
                binding.AttachBrowser(core.BrowserProcessId, runtime);
                InstallDenials(core);
                var settings = core.Settings;
                settings.IsWebMessageEnabled = false; settings.AreHostObjectsAllowed = false;
                settings.AreDevToolsEnabled = false; settings.AreBrowserAcceleratorKeysEnabled = false;
                settings.AreDefaultContextMenusEnabled = false; settings.AreDefaultScriptDialogsEnabled = false;
                settings.IsGeneralAutofillEnabled = false; settings.IsPasswordAutosaveEnabled = false;
                settings.IsReputationCheckingRequired = true;
                if (settings.IsWebMessageEnabled || settings.AreHostObjectsAllowed || settings.AreDevToolsEnabled || settings.IsGeneralAutofillEnabled || settings.IsPasswordAutosaveEnabled || !settings.IsReputationCheckingRequired || !core.Profile.IsInPrivateModeEnabled || core.Profile.ProfileName != options.ProfileId) throw LaunchOptions.Reject("browser_settings_readback_failed");

                // Set the native engine override before any owned document is
                // created. Unsupported experimental CDP methods fail closed.
                await options.Hardware.ConfigureTarget((method, parameters) => Cdp(core, null, method, parameters));

                if (options.Graphics.Metadata)
                {
                    string scriptId = await Deadline(core.AddScriptToExecuteOnDocumentCreatedAsync(GraphicsScripts.Install(options.Graphics)));
                    if (String.IsNullOrEmpty(scriptId)) throw LaunchOptions.Reject("graphics_script_registration_failed");
                }

                // An app-owned, in-memory HTTPS response establishes a secure
                // context for native Client Hints observation. The closed relay
                // and request interceptor deny every network request meanwhile.
                bootstrap = true; bootstrapComplete = new TaskCompletionSource<bool>();
                core.Navigate(Bootstrap); await Deadline(bootstrapComplete.Task);
                nativeUserAgent = settings.UserAgent;
                nativeHints = await ReadNativeHints(core);
                var nativeProbe = EvaluationValue(await Cdp(core, null, "Runtime.evaluate", new { expression = "({platform:navigator.platform,userAgent:navigator.userAgent})", returnByValue = true }));
                nativePlatform = LaunchOptions.Text(nativeProbe, "platform");
                if (LaunchOptions.Text(nativeProbe, "userAgent") != nativeUserAgent) throw LaunchOptions.Reject("native_ua_source_mismatch");
                identity = RuntimeIdentity.Derive(options.Environment, nativeUserAgent, nativeHints, nativeModel, nativePlatform, environment.BrowserVersionString, options.IdentityMode, options.UserAgent);
                string graphicsStatus = GraphicsScripts.Validate(EvaluationValue(await Cdp(core, null, "Runtime.evaluate", new { expression = GraphicsScripts.Probe(options.Graphics.Metadata, options.WebGpuDisabled), awaitPromise = true, returnByValue = true })), options.Graphics, options.WebGpuDisabled);
                // App-owned test frames are removed before related-target
                // auto-attach is enabled. They are never remote website frames.
                await Cdp(core, null, "Runtime.evaluate", new { expression = "document.querySelectorAll('iframe').forEach(frame => frame.remove())", returnByValue = true });
                if (options.IdentityMode == "observed-exact")
                {
                    var expected = new Dictionary<string, object>(options.ClientHints);
                    if (expected.ContainsKey("model")) { if (LaunchOptions.Text(expected, "model") != nativeModel) throw LaunchOptions.Reject("observed_model_changed"); expected.Remove("model"); }
                    if (options.UserAgent != nativeUserAgent || !DeepEqual(expected, nativeHints)) throw LaunchOptions.Reject("observed_identity_changed");
                }
                settings.UserAgent = identity.UserAgent;
                if (settings.UserAgent != identity.UserAgent) throw LaunchOptions.Reject("ua_readback_failed");
                // The official UserAgent property may clear navigator UA-CH.
                // Restore explicit live-derived metadata through CDP, then
                // verify script values and a real app-owned request separately.
                await ConfigureTarget(core, null);
                await VerifyUserAgentRequest(core);
                await ConfigureTarget(core, null);
                if (options.Hardware.Custom)
                {
                    options.Hardware.ValidateFrames(HardwareOptions.EvaluationValue(await Cdp(core, null, "Runtime.evaluate", new { expression = HardwareOptions.ProbeFrames, returnByValue = true })));
                    // As with the owned GPU probe, remove these frames before
                    // attaching related targets. Never resume an existing frame
                    // that the protocol did not pause before first execution.
                    await Cdp(core, null, "Runtime.evaluate", new { expression = "document.querySelectorAll('iframe').forEach(frame => frame.remove())", returnByValue = true });
                }
                targets = core.GetDevToolsProtocolEventReceiver("Target.attachedToTarget");
                targets.DevToolsProtocolEventReceived += async delegate(object sender, CoreWebView2DevToolsProtocolEventReceivedEventArgs e)
                {
                    try
                    {
                        var message = LaunchOptions.Object(LaunchOptions.Json.DeserializeObject(e.ParameterObjectAsJson));
                        if (!LaunchOptions.Boolean(message, "waitingForDebugger")) throw LaunchOptions.Reject("target_not_paused");
                        string session = LaunchOptions.Text(message, "sessionId");
                        var target = LaunchOptions.Object(LaunchOptions.Value(message, "targetInfo"));
                        // Worker-specific emulation coverage is not implemented.
                        // Paused workers are terminated, never silently resumed.
                        if (LaunchOptions.Text(target, "type") != "iframe")
                        {
                            await Cdp(core, null, "Target.closeTarget", new { targetId = LaunchOptions.Text(target, "targetId") });
                            return;
                        }
                        await ConfigureTarget(core, session);
                        await Cdp(core, session, "Target.setAutoAttach", new { autoAttach = true, waitForDebuggerOnStart = true, flatten = true });
                        if (revoked) return;
                        await Cdp(core, session, "Runtime.runIfWaitingForDebugger", new { });
                    }
                    catch { Revoke(); }
                };
                await Cdp(core, null, "Target.setAutoAttach", new { autoAttach = true, waitForDebuggerOnStart = true, flatten = true });
                WindowsGuard.AssertNoOverrides(); binding.CheckAlive();
                if (revoked || !lease.Live) { Revoke(); return; }
                bootstrap = false; relay.Open(); gate.Configured();
                address.Enabled = go.Enabled = reload.Enabled = true; view.Visible = true;
                summary.Text += options.Environment.Emulated ? "\n网页环境模拟：" + options.Environment.OS + " " + options.Environment.OSVersion + "；实际仍为 Windows Chromium/WebView2，可被检测。" : "\n网页环境：原生 Windows Chromium/WebView2。";
                state.Text = (options.ProfileProxy ? "自定义代理叠加 Ultra；" : "Ultra SOCKS5；") + "绑定与网页设置已核验。" + graphicsStatus;
                Console.Out.WriteLine("{\"type\":\"ready\",\"schemaVersion\":1}"); Console.Out.Flush();
                WasReady = true;
            }
            catch
            {
                FailureReceipt.CloseThenReport(Revoke, Console.Out, "configuration_refused");
            }
        }

        private void InstallDenials(CoreWebView2 core)
        {
            core.NavigationStarting += delegate(object sender, CoreWebView2NavigationStartingEventArgs e)
            {
                if (!(bootstrap && (e.Uri == Bootstrap || identityProbe && e.Uri == BootstrapIdentity)) && (!gate.Allowed || !LaunchOptions.NetworkUri(e.Uri))) e.Cancel = true;
            };
            core.FrameNavigationStarting += delegate(object sender, CoreWebView2NavigationStartingEventArgs e) { if (!(bootstrap && (e.Uri == BootstrapFrame || e.Uri == "about:srcdoc" || e.Uri == "about:blank")) && (!gate.Allowed || !LaunchOptions.NetworkUri(e.Uri))) e.Cancel = true; };
            core.PermissionRequested += delegate(object sender, CoreWebView2PermissionRequestedEventArgs e) { e.State = CoreWebView2PermissionState.Deny; e.SavesInProfile = false; e.Handled = true; };
            core.NewWindowRequested += delegate(object sender, CoreWebView2NewWindowRequestedEventArgs e) { e.Handled = true; };
            core.DownloadStarting += delegate(object sender, CoreWebView2DownloadStartingEventArgs e) { e.Cancel = true; e.Handled = true; };
            core.ScreenCaptureStarting += delegate(object sender, CoreWebView2ScreenCaptureStartingEventArgs e) { e.Cancel = true; e.Handled = true; };
            core.SaveAsUIShowing += delegate(object sender, CoreWebView2SaveAsUIShowingEventArgs e) { e.Cancel = true; e.SuppressDefaultDialog = true; };
            core.SaveFileSecurityCheckStarting += delegate(object sender, CoreWebView2SaveFileSecurityCheckStartingEventArgs e) { e.CancelSave = true; };
            core.LaunchingExternalUriScheme += delegate(object sender, CoreWebView2LaunchingExternalUriSchemeEventArgs e) { e.Cancel = true; };
            core.BasicAuthenticationRequested += delegate(object sender, CoreWebView2BasicAuthenticationRequestedEventArgs e) { e.Cancel = true; };
            core.ClientCertificateRequested += delegate(object sender, CoreWebView2ClientCertificateRequestedEventArgs e) { e.Cancel = true; e.Handled = true; };
            core.ServerCertificateErrorDetected += delegate(object sender, CoreWebView2ServerCertificateErrorDetectedEventArgs e) { e.Action = CoreWebView2ServerCertificateErrorAction.Cancel; };
            core.ProcessFailed += delegate { Revoke(); };
            core.WindowCloseRequested += delegate { Revoke(); };
            core.NavigationCompleted += delegate(object sender, CoreWebView2NavigationCompletedEventArgs e)
            {
                if (bootstrap && bootstrapComplete != null) { if (e.IsSuccess) bootstrapComplete.TrySetResult(true); else bootstrapComplete.TrySetException(LaunchOptions.Reject("local_bootstrap_failed")); }
                else if (!e.IsSuccess) state.Text = "导航失败；会话不会回退为直连。";
            };
            core.HistoryChanged += delegate { if (gate.Allowed) { back.Enabled = core.CanGoBack; forward.Enabled = core.CanGoForward; } };
            core.SourceChanged += delegate { if (gate.Allowed && LaunchOptions.NetworkUri(core.Source)) address.Text = core.Source; };
            core.AddWebResourceRequestedFilter("*", CoreWebView2WebResourceContext.All, CoreWebView2WebResourceRequestSourceKinds.All);
            core.WebResourceRequested += delegate(object sender, CoreWebView2WebResourceRequestedEventArgs e)
            {
                try
                {
                    if (bootstrap && (e.Request.Uri == Bootstrap || e.Request.Uri == BootstrapFrame || identityProbe && e.Request.Uri == BootstrapIdentity))
                    {
                        string content = "<!doctype html><meta charset=utf-8><meta name=viewport content='width=device-width,initial-scale=1'><title>Ultra private session</title>";
                        if (identity != null) RuntimeIdentity.VerifyUserAgentHeader(e.Request.Headers.GetHeader("User-Agent"), identity.UserAgent);
                        if (e.Request.Uri == Bootstrap && !identityProbe && (options.Graphics.Metadata || options.WebGpuDisabled)) content += "<iframe id='gpu-same-origin' src='" + BootstrapFrame + "'></iframe><iframe id='gpu-srcdoc' srcdoc='<!doctype html><title>Owned graphics probe</title>'></iframe><iframe id='gpu-aboutblank' src='about:blank'></iframe>";
                        if (e.Request.Uri == BootstrapIdentity && identityProbe && options.Hardware.Custom) content += "<iframe id='cpu-same-origin' src='" + BootstrapFrame + "'></iframe><iframe id='cpu-srcdoc' srcdoc='<!doctype html><title>Owned CPU readback</title>'></iframe><iframe id='cpu-aboutblank' src='about:blank'></iframe>";
                        var html = new MemoryStream(Encoding.UTF8.GetBytes(content));
                        e.Response = environment.CreateWebResourceResponse(html, 200, "OK", "Content-Type: text/html; charset=utf-8\r\nCache-Control: no-store\r\nContent-Security-Policy: default-src 'none'; frame-src 'self' about:");
                    }
                    else if (!gate.Allowed || !LaunchOptions.NetworkUri(e.Request.Uri)) e.Response = environment.CreateWebResourceResponse(null, 403, "Blocked", "Cache-Control: no-store");
                    else RuntimeIdentity.VerifyUserAgentHeader(e.Request.Headers.GetHeader("User-Agent"), identity.UserAgent);
                }
                catch { Revoke(); }
            };
        }

        private async Task<IDictionary<string, object>> ReadNativeHints(CoreWebView2 core)
        {
            var response = await Cdp(core, null, "Runtime.evaluate", new { expression = "navigator.userAgentData.getHighEntropyValues(['architecture','bitness','platformVersion','fullVersionList','model'])", awaitPromise = true, returnByValue = true });
            var value = EvaluationValue(response);
            nativeModel = LaunchOptions.Text(value, "model"); value.Remove("model");
            LaunchOptions.Keys(value, "brands", "fullVersionList", "platform", "platformVersion", "architecture", "bitness", "mobile");
            if (LaunchOptions.Text(value, "platform") != "Windows" || LaunchOptions.Boolean(value, "mobile")) throw LaunchOptions.Reject("unsupported_native_identity");
            return value;
        }
        private async Task ConfigureTarget(CoreWebView2 core, string session)
        {
            await options.Hardware.ConfigureTarget((method, parameters) => Cdp(core, session, method, parameters));
            await Cdp(core, session, "Network.enable", new { });
            await Cdp(core, session, "Network.setUserAgentOverride", new { userAgent = identity.UserAgent, acceptLanguage = options.AcceptLanguage, platform = identity.NavigatorPlatform, userAgentMetadata = identity.Hints });
            await Cdp(core, session, "Emulation.setLocaleOverride", new { locale = options.Locale });
            await Cdp(core, session, "Emulation.setTimezoneOverride", new { timezoneId = options.Timezone });
            if (options.Environment.Emulated)
            {
                if (session == null) await Cdp(core, session, "Emulation.setDeviceMetricsOverride", Metrics());
                await Cdp(core, session, "Emulation.setTouchEmulationEnabled", new { enabled = options.Environment.Mobile, maxTouchPoints = options.Environment.Mobile ? options.Environment.TouchPoints : 1 });
            }
            var result = await Cdp(core, session, "Runtime.evaluate", new { expression = "(async()=>({userAgent:navigator.userAgent,platform:navigator.platform,language:navigator.language,languages:Array.from(navigator.languages),timezone:Intl.DateTimeFormat().resolvedOptions().timeZone,locale:Intl.NumberFormat().resolvedOptions().locale,width:innerWidth,height:innerHeight,screenWidth:screen.width,screenHeight:screen.height,dpr:devicePixelRatio,touch:navigator.maxTouchPoints,hints:await navigator.userAgentData.getHighEntropyValues(['architecture','bitness','platformVersion','fullVersionList','model'])}))()", awaitPromise = true, returnByValue = true });
            var value = EvaluationValue(result);
            if (LaunchOptions.Text(value, "userAgent") != identity.UserAgent || LaunchOptions.Text(value, "platform") != identity.NavigatorPlatform || LaunchOptions.Text(value, "language") != options.Language || LaunchOptions.Text(value, "timezone") != options.Timezone || LaunchOptions.Text(value, "locale") != options.Locale || !DeepEqual(LaunchOptions.Object(LaunchOptions.Value(value, "hints")), identity.Hints)) throw LaunchOptions.Reject("emulation_readback_failed");
            if (options.Environment.Emulated && (Convert.ToDouble(LaunchOptions.Value(value, "screenWidth")) != options.Width || Convert.ToDouble(LaunchOptions.Value(value, "screenHeight")) != options.Height || Math.Abs(Convert.ToDouble(LaunchOptions.Value(value, "dpr")) - options.Environment.DPR) > 0.00001 || Convert.ToInt32(LaunchOptions.Value(value, "touch")) != options.Environment.TouchPoints)) throw LaunchOptions.Reject("device_metrics_readback_failed");
            if (options.Environment.Emulated && session == null && (Convert.ToDouble(LaunchOptions.Value(value, "width")) != options.Width || Convert.ToDouble(LaunchOptions.Value(value, "height")) != options.Height)) throw LaunchOptions.Reject("viewport_readback_failed");
            var languages = LaunchOptions.Value(value, "languages") as object[];
            if (languages == null || String.Join(",", languages) != options.AcceptLanguage) throw LaunchOptions.Reject("language_readback_failed");
        }
        private async Task VerifyUserAgentRequest(CoreWebView2 core)
        {
            // The only request allowed here receives an app-owned in-memory
            // response. The relay remains closed: no probe URL reaches a server.
            var observed = new TaskCompletionSource<bool>();
            var receiver = core.GetDevToolsProtocolEventReceiver("Network.requestWillBeSent");
            EventHandler<CoreWebView2DevToolsProtocolEventReceivedEventArgs> handler = delegate(object sender, CoreWebView2DevToolsProtocolEventReceivedEventArgs e)
            {
                try
                {
                    var value = LaunchOptions.Object(LaunchOptions.Json.DeserializeObject(e.ParameterObjectAsJson));
                    var request = LaunchOptions.Object(LaunchOptions.Value(value, "request"));
                    if (LaunchOptions.Text(request, "url") != BootstrapIdentity) return;
                    RuntimeIdentity.VerifyUserAgentHeaders(LaunchOptions.Object(LaunchOptions.Value(request, "headers")), identity.UserAgent);
                    observed.TrySetResult(true);
                }
                catch { observed.TrySetException(LaunchOptions.Reject("ua_http_readback_failed")); }
            };
            receiver.DevToolsProtocolEventReceived += handler;
            try
            {
                identityProbe = true; bootstrapComplete = new TaskCompletionSource<bool>();
                core.Navigate(BootstrapIdentity);
                await Deadline(Task.WhenAll(observed.Task, bootstrapComplete.Task));
            }
            finally { identityProbe = false; receiver.DevToolsProtocolEventReceived -= handler; }
        }
        private object Metrics()
        { return RuntimeIdentity.Metrics(options.Environment, options.Width, options.Height, displayScale); }
        private Task<IDisposable> AuthorizeFrontend(TcpClient client)
        {
            var completion = new TaskCompletionSource<IDisposable>();
            try
            {
                BeginInvoke((Action)delegate
                {
                    try
                    {
                        if (revoked || environment == null || view.CoreWebView2 == null) throw LaunchOptions.Reject("frontend_session_closed");
                        var allowed = new HashSet<int>();
                        foreach (var process in environment.GetProcessInfos())
                            if (process.Kind == CoreWebView2ProcessKind.Browser || process.Kind == CoreWebView2ProcessKind.Utility || process.Kind == CoreWebView2ProcessKind.Renderer) allowed.Add(process.ProcessId);
                        IDisposable pinned = binding.CheckFrontendSocket(client, allowed); if (!completion.TrySetResult(pinned)) pinned.Dispose();
                    }
                    catch { completion.TrySetException(LaunchOptions.Reject("frontend_not_this_webview")); }
                });
            }
            catch { completion.TrySetException(LaunchOptions.Reject("frontend_session_closed")); }
            return completion.Task;
        }
        private static IDictionary<string, object> EvaluationValue(IDictionary<string, object> response)
        {
            if (response.ContainsKey("exceptionDetails")) throw LaunchOptions.Reject("runtime_evaluation_failed");
            return LaunchOptions.Object(LaunchOptions.Value(LaunchOptions.Object(LaunchOptions.Value(response, "result")), "value"));
        }
        private async Task<IDictionary<string, object>> Cdp(CoreWebView2 core, string session, string method, object parameters)
        {
            if (revoked) throw LaunchOptions.Reject("session_revoked");
            string json = LaunchOptions.Json.Serialize(parameters);
            string response = await Deadline(session == null ? core.CallDevToolsProtocolMethodAsync(method, json) : core.CallDevToolsProtocolMethodForSessionAsync(session, method, json));
            if (revoked) throw LaunchOptions.Reject("session_revoked");
            var value = LaunchOptions.Object(LaunchOptions.Json.DeserializeObject(response));
            if (value.ContainsKey("error")) throw LaunchOptions.Reject("cdp_rejected");
            return value;
        }
        private void NavigateAddress()
        {
            try { binding.CheckAlive(); if (!lease.Live) { Revoke(); return; } if (!gate.Allowed || !LaunchOptions.NetworkUri(address.Text)) { state.Text = "请输入不含内嵌凭据的完整 HTTP 或 HTTPS 地址。"; return; } view.CoreWebView2.Navigate(address.Text); }
            catch { Revoke(); }
        }
        private void BrowserCommand(string command)
        {
            try
            {
                binding.CheckAlive();
                if (!lease.Live || !gate.Allowed) { Revoke(); return; }
                var core = view.CoreWebView2;
                if (command == "back" && core.CanGoBack) core.GoBack();
                else if (command == "forward" && core.CanGoForward) core.GoForward();
                else if (command == "reload") core.Reload();
            }
            catch { Revoke(); }
        }
        private static async Task<T> Deadline<T>(Task<T> operation)
        {
            if (await Task.WhenAny(operation, Task.Delay(10000)) != operation) throw LaunchOptions.Reject("browser_operation_timeout");
            return await operation;
        }
        private static async Task Deadline(Task operation)
        {
            if (await Task.WhenAny(operation, Task.Delay(10000)) != operation) throw LaunchOptions.Reject("browser_operation_timeout");
            await operation;
        }
        private static bool DeepEqual(object a, object b)
        {
            var left = a as IDictionary<string, object>; var right = b as IDictionary<string, object>;
            if (left != null && right != null) { if (left.Count != right.Count) return false; foreach (var pair in left) { object value; if (!right.TryGetValue(pair.Key, out value) || !DeepEqual(pair.Value, value)) return false; } return true; }
            var la = a as object[]; var ra = b as object[];
            if (la != null && ra != null) { if (la.Length != ra.Length) return false; for (int n = 0; n < la.Length; n++) if (!DeepEqual(la[n], ra[n])) return false; return true; }
            return Object.Equals(a, b);
        }
        protected override void Dispose(bool disposing)
        {
            if (disposing) { revoked = true; gate.Close(); if (relay != null) relay.Dispose(); watcher.Dispose(); view.Dispose(); if (dataLease != null) dataLease.Dispose(); }
            base.Dispose(disposing);
        }
    }
}
