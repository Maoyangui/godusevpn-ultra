using System;
using System.IO;
using System.Text;
using System.Threading.Tasks;

namespace GodUseVPN.Ultra.BrowserHost
{
    // Only the browser-facing no-auth greeting is translated. CONNECT and all
    // subsequent bytes remain SOCKS5; no DNS resolution or DIRECT route exists.
    internal static class RelayProtocol
    {
        internal static async Task<byte[]> ReadExact(Stream stream, int count)
        {
            var data = new byte[count]; int offset = 0;
            while (offset < count) { Task<int> read = stream.ReadAsync(data, offset, count - offset); if (await Task.WhenAny(read, Task.Delay(3000)).ConfigureAwait(false) != read) throw LaunchOptions.Reject("socks_handshake_timeout"); int n = await read.ConfigureAwait(false); if (n == 0) throw LaunchOptions.Reject("socks_handshake_ended"); offset += n; }
            return data;
        }
        private static async Task Send(Stream stream, byte[] data)
        { Task write = stream.WriteAsync(data, 0, data.Length); if (await Task.WhenAny(write, Task.Delay(3000)).ConfigureAwait(false) != write) throw LaunchOptions.Reject("socks_handshake_timeout"); await write.ConfigureAwait(false); }
        internal static async Task FrontendGreeting(Stream stream)
        {
            byte[] header = await ReadExact(stream, 2).ConfigureAwait(false); if (header[0] != 5 || header[1] < 1 || header[1] > 16) throw LaunchOptions.Reject("frontend_socks_methods_invalid");
            byte[] methods = await ReadExact(stream, header[1]).ConfigureAwait(false); if (Array.IndexOf(methods, (byte)0) < 0) throw LaunchOptions.Reject("frontend_noauth_unavailable");
        }
        internal static async Task UpstreamGreeting(Stream stream, string username, string password)
        {
            bool authenticated = username.Length != 0 || password.Length != 0; byte method = authenticated ? (byte)2 : (byte)0;
            await Send(stream, new byte[] { 5, 1, method }).ConfigureAwait(false);
            byte[] selection = await ReadExact(stream, 2).ConfigureAwait(false); if (selection[0] != 5 || selection[1] != method) throw LaunchOptions.Reject("upstream_socks_method_refused");
            if (!authenticated) return;
            byte[] user = Encoding.ASCII.GetBytes(username), secret = Encoding.ASCII.GetBytes(password); if (user.Length < 1 || user.Length > 255 || secret.Length < 1 || secret.Length > 255) throw LaunchOptions.Reject("upstream_credentials_invalid");
            var frame = new byte[3 + user.Length + secret.Length]; frame[0] = 1; frame[1] = (byte)user.Length; Array.Copy(user, 0, frame, 2, user.Length); frame[2 + user.Length] = (byte)secret.Length; Array.Copy(secret, 0, frame, 3 + user.Length, secret.Length);
            try { await Send(stream, frame).ConfigureAwait(false); } finally { Array.Clear(frame, 0, frame.Length); Array.Clear(secret, 0, secret.Length); }
            byte[] status = await ReadExact(stream, 2).ConfigureAwait(false); if (status[0] != 1 || status[1] != 0) throw LaunchOptions.Reject("upstream_authentication_refused");
        }
        internal static Task FrontendAccepted(Stream stream) { return Send(stream, new byte[] { 5, 0 }); }
    }
}
