using System;
using System.Collections.Generic;
using System.Net;
using System.Net.Sockets;
using System.Threading;
using System.Threading.Tasks;

namespace GodUseVPN.Ultra.BrowserHost
{
    // The engine only knows this session-owned endpoint. It never receives the
    // upstream port, a system proxy, a PAC URL, or a DIRECT alternative.
    internal sealed class SocksRelay : IDisposable
    {
        private readonly TcpListener listener;
        private readonly WindowsGuard binding;
        private readonly int upstreamPort;
        private readonly string username, password;
        private readonly Func<TcpClient, Task<IDisposable>> authorize;
        private readonly Action failed;
        private readonly object sync = new object();
        private readonly HashSet<TcpClient> clients = new HashSet<TcpClient>();
        private bool opened, stopped;
        internal int Port { get { return ((IPEndPoint)listener.LocalEndpoint).Port; } }
        internal SocksRelay(WindowsGuard guard, LaunchOptions options, Func<TcpClient, Task<IDisposable>> frontendAuthorization, Action onFailure)
        {
            binding = guard; upstreamPort = options.ProxyPort; username = options.RelayUsername; password = options.RelayPassword; authorize = frontendAuthorization; failed = onFailure;
            listener = new TcpListener(IPAddress.Loopback, 0); listener.ExclusiveAddressUse = true; listener.Start(32);
            Task.Run((Func<Task>)Accept);
        }
        internal void Open()
        {
            binding.CheckAlive();
            lock (sync) { if (stopped) throw LaunchOptions.Reject("relay_closed"); opened = true; }
        }
        private async Task Accept()
        {
            try
            {
                while (true)
                {
                    TcpClient incoming = await listener.AcceptTcpClientAsync().ConfigureAwait(false);
                    lock (sync)
                    {
                        if (stopped || !opened || clients.Count >= 128) { incoming.Close(); continue; }
                        clients.Add(incoming);
                    }
                    // Each connection has a bounded local connect and its own
                    // server-side owner check before any browser bytes are sent.
                    RunRelay(incoming);
                }
            }
            catch { lock (sync) { if (stopped) return; } Dispose(); failed(); }
        }
        private void RunRelay(TcpClient incoming) { Task.Run(() => Relay(incoming)); }
        private async Task Relay(TcpClient incoming)
        {
            var upstream = new TcpClient(AddressFamily.InterNetwork);
            bool frontendVerified = false, upstreamReady = false;
            IDisposable frontend = null;
            try
            {
                // The public loopback endpoint is not a bearer secret. Verify
                // its real client process before sending the Go relay password.
                Task<IDisposable> authorization = authorize(incoming);
                if (await Task.WhenAny(authorization, Task.Delay(3000)).ConfigureAwait(false) != authorization)
                {
                    DisposeLateAuthorization(authorization);
                    throw LaunchOptions.Reject("frontend_authorization_timeout");
                }
                frontend = await authorization.ConfigureAwait(false); frontendVerified = true;
                await RelayProtocol.FrontendGreeting(incoming.GetStream()).ConfigureAwait(false);
                lock (sync) { if (stopped || !opened) return; clients.Add(upstream); }
                binding.CheckAlive();
                Task connection = upstream.ConnectAsync(IPAddress.Loopback, upstreamPort);
                if (await Task.WhenAny(connection, Task.Delay(3000)).ConfigureAwait(false) != connection) throw LaunchOptions.Reject("upstream_timeout");
                await connection.ConfigureAwait(false);
                binding.CheckConnectedSocket(upstream);
                lock (sync) { if (stopped || !opened) return; }
                using (var from = incoming.GetStream()) using (var to = upstream.GetStream())
                {
                    await RelayProtocol.UpstreamGreeting(to, username, password).ConfigureAwait(false);
                    binding.CheckConnectedSocket(upstream);
                    lock (sync) { if (stopped || !opened) return; }
                    await RelayProtocol.FrontendAccepted(from).ConfigureAwait(false); upstreamReady = true;
                    Task send = from.CopyToAsync(to), receive = to.CopyToAsync(from);
                    Task first = await Task.WhenAny(send, receive).ConfigureAwait(false);
                    await first.ConfigureAwait(false);
                }
            }
            catch { if (frontendVerified && !upstreamReady) { Dispose(); failed(); } }
            finally
            {
                incoming.Close(); upstream.Close();
                if (frontend != null) frontend.Dispose();
                lock (sync) { clients.Remove(incoming); clients.Remove(upstream); }
            }
        }
        private static void DisposeLateAuthorization(Task<IDisposable> authorization)
        {
            authorization.ContinueWith(task =>
            {
                if (task.Status == TaskStatus.RanToCompletion) task.Result.Dispose();
                else if (task.IsFaulted) GC.KeepAlive(task.Exception);
            });
        }
        public void Dispose()
        {
            lock (sync)
            {
                if (stopped) return; stopped = true; opened = false;
                listener.Stop(); foreach (var client in clients) client.Close(); clients.Clear();
            }
        }
    }
}
