using System;
using System.IO;
using System.Text;
using System.Threading.Tasks;
using System.Windows.Forms;

namespace GodUseVPN.Ultra.BrowserHost
{
    internal static class Program
    {
        [STAThread]
        private static int Main(string[] args)
        {
            try
            {
                Console.InputEncoding = new UTF8Encoding(false, true); Console.OutputEncoding = new UTF8Encoding(false);
                WindowsGuard.AssertOrdinaryToken();
                if (args.Length != 0 || !Console.IsInputRedirected) throw LaunchOptions.Reject("trusted_launch_pipe_required");
                Task<string> read = Task.Run(() => ReadLine(Console.In, 32768));
                if (!read.Wait(10000)) throw LaunchOptions.Reject("launch_frame_timeout");
                LaunchOptions options = LaunchOptions.Parse(read.Result);
                using (var guard = new WindowsGuard(options))
                {
                    WindowsGuard.AssertNoOverrides();
                    string runtime = WindowsGuard.RuntimeDirectory();
                    Microsoft.Web.WebView2.Core.CoreWebView2Environment.SetLoaderDllFolderPath(AppDomain.CurrentDomain.BaseDirectory);
                    Application.SetUnhandledExceptionMode(UnhandledExceptionMode.ThrowException);
                    Application.EnableVisualStyles(); Application.SetCompatibleTextRenderingDefault(false);
                    var lease = new PipeLease();
                    using (var form = new BrowserForm(options, guard, runtime, lease))
                    {
                        // This pipe is the caller's *ongoing* connected/strict
                        // lease. The Go parent must close it on any state loss.
                        // Fixed lease frames refresh this connection for at most
                        // three seconds. EOF/close/unknown input is terminal.
                        Task.Run(() => { try { while (lease.Frame(ReadLine(Console.In, 128))) { } } catch { } finally { lease.Close(); form.Revoke(); } });
                        // Independent of the WinForms/COM message pump: a stuck
                        // UI cannot keep the network relay open after expiry.
                        Task.Run(async delegate { while (lease.Live) await Task.Delay(200); form.Revoke(); });
                        try { Application.Run(form); } finally { lease.Close(); }
                        if (!form.WasReady) return 1;
                    }
                }
                return 0;
            }
            catch (Exception)
            {
                // Never print configuration, URL, profile path or credentials.
                FailureReceipt.CloseThenReport(delegate { }, Console.Out, "start_refused");
                return 1;
            }
        }
        private static string ReadLine(TextReader reader, int limit)
        {
            var text = new StringBuilder();
            while (true)
            {
                int next = reader.Read();
                if (next < 0) throw LaunchOptions.Reject("launch_pipe_ended");
                if (next == '\n') return text.ToString().TrimEnd('\r');
                if (text.Length >= limit) throw LaunchOptions.Reject("launch_size");
                text.Append((char)next);
            }
        }
    }
}
