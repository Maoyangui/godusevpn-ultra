using System;
using System.Collections.Generic;
using System.Globalization;
using System.Runtime.InteropServices;
using System.Security.Principal;
using System.Threading;
using System.Web.Script.Serialization;
using System.Windows.Forms;
using System.Drawing;
using Windows.Devices.Geolocation;
using Windows.Foundation;

// This executable is an independent, read-only consumer of Windows location.
// No portal URI, target coordinates, process path or arbitrary action is input.
internal static class LocationConsumer
{
    [DllImport("advapi32.dll", SetLastError=true)] static extern bool GetTokenInformation(IntPtr token, int kind, IntPtr buffer, int length, out int used);
    [DllImport("advapi32.dll", SetLastError=true)] static extern bool OpenThreadToken(IntPtr thread, uint access, bool asSelf, out IntPtr token);
    [DllImport("kernel32.dll")] static extern IntPtr GetCurrentThread();
    [DllImport("kernel32.dll")] static extern bool CloseHandle(IntPtr handle);
    [DllImport("advapi32.dll")] static extern IntPtr GetSidSubAuthorityCount(IntPtr sid);
    [DllImport("advapi32.dll")] static extern IntPtr GetSidSubAuthority(IntPtr sid, uint index);
    internal static bool Finite(double value) { return !double.IsNaN(value) && !double.IsInfinity(value); }
    internal static bool ValidObservation(double latitude, double longitude, double accuracy, int source, DateTimeOffset timestamp, DateTimeOffset after, DateTimeOffset now)
    {
        return Finite(latitude) && latitude>=-90 && latitude<=90 && Finite(longitude) && longitude>=-180 && longitude<=180 && Finite(accuracy) && accuracy>0 && accuracy<=1000 && source==5 && timestamp>=after && timestamp<=now.AddSeconds(2) && timestamp>=now.AddSeconds(-15);
    }
    static bool OrdinaryUser()
    {
        IntPtr thread; bool opened=OpenThreadToken(GetCurrentThread(),8,true,out thread); int code=Marshal.GetLastWin32Error();
        if(opened) { CloseHandle(thread); return false; } if(code!=1008)return false;
        using(WindowsIdentity identity=WindowsIdentity.GetCurrent())
        {
            if(identity.IsSystem || identity.User==null || identity.User.Value=="S-1-5-19" || identity.User.Value=="S-1-5-20")return false;
            IntPtr buffer=Marshal.AllocHGlobal(512);
            try
            {
                int used; if(!GetTokenInformation(identity.Token,20,buffer,4,out used)||used!=4||Marshal.ReadInt32(buffer)!=0)return false;
                if(!GetTokenInformation(identity.Token,25,buffer,512,out used)||used< IntPtr.Size+4 || used>512)return false;
                IntPtr sid=Marshal.ReadIntPtr(buffer); if(sid==IntPtr.Zero)return false;
                int count=Marshal.ReadByte(GetSidSubAuthorityCount(sid)); if(count<1||count>15)return false;
                int integrity=Marshal.ReadInt32(GetSidSubAuthority(sid,(uint)(count-1)));return integrity==8192||integrity==8448;
            }
            finally { Marshal.FreeHGlobal(buffer); }
        }
    }
    internal static bool ReadRequest(string line, out DateTimeOffset after, out int timeout)
    {
        after=DateTimeOffset.MinValue;timeout=0;if(line==null||line.Length>1024)return false;
        // Canonical, fixed frame emitted by the trusted Go caller. Rejecting
        // all whitespace/alternate JSON forms also rejects duplicate keys.
        const string prefix="{\"schemaVersion\":1,\"requestedAfter\":\"";const string middle="\",\"timeoutSeconds\":";
        if(!line.StartsWith(prefix,StringComparison.Ordinal)||!line.EndsWith("}",StringComparison.Ordinal))return false;
        int split=line.IndexOf(middle,prefix.Length,StringComparison.Ordinal);if(split<0)return false;
        string stamp=line.Substring(prefix.Length,split-prefix.Length);string seconds=line.Substring(split+middle.Length,line.Length-split-middle.Length-1);
        if(!int.TryParse(seconds,NumberStyles.None,CultureInfo.InvariantCulture,out timeout)||timeout<2||timeout>12||seconds!=timeout.ToString(CultureInfo.InvariantCulture))return false;
        if(!DateTimeOffset.TryParse(stamp,CultureInfo.InvariantCulture,DateTimeStyles.AssumeUniversal|DateTimeStyles.AdjustToUniversal,out after)||stamp.Length>40||stamp.IndexOf('T')<0||!stamp.EndsWith("Z",StringComparison.Ordinal))return false;
        DateTimeOffset now=DateTimeOffset.UtcNow;return after>=now.AddSeconds(-5)&&after<=now.AddSeconds(2);
    }
    static void Reply(object value) { string text=new JavaScriptSerializer().Serialize(value);if(text.Length>4096)throw new InvalidOperationException();Console.Out.WriteLine(text);Console.Out.Flush(); }
    sealed class ReadForm : Form
    {
        [DllImport("user32.dll")] static extern IntPtr GetForegroundWindow();
        readonly DateTimeOffset after;
        readonly int timeout;
        readonly System.Windows.Forms.Timer timer=new System.Windows.Forms.Timer();
        readonly Label status=new Label();
        IAsyncOperation<GeolocationAccessStatus> access;
        IAsyncOperation<Geoposition> position;
        DateTimeOffset deadline;
        DateTimeOffset readAfter;
        bool requested;
        internal object Result;
        internal ReadForm(DateTimeOffset requestedAfter,int timeoutSeconds)
        {
            after=requestedAfter;timeout=timeoutSeconds;Text="GodUseVPN Ultra · 独立系统位置读取";ClientSize=new Size(390,155);MinimumSize=Size;MaximumSize=Size;StartPosition=FormStartPosition.CenterScreen;MaximizeBox=false;MinimizeBox=false;
            status.SetBounds(18,16,354,78);status.Text="准备请求 Windows 定位权限。\n仅请求独立读取；是否允许由 Windows 提示与系统设置决定。\n可随时取消；首次授权最多等待 90 秒。";Controls.Add(status);
            Button cancel=new Button();cancel.Text="取消读取";cancel.SetBounds(260,108,110,30);cancel.Click+=delegate{Close();};Controls.Add(cancel);CancelButton=cancel;
            timer.Interval=25;timer.Tick+=Tick;
            Shown+=delegate{deadline=DateTimeOffset.UtcNow.AddSeconds(90);Activate();timer.Start();};
            FormClosed+=delegate{timer.Stop();timer.Dispose();CloseOperation(access);CloseOperation(position);};
        }
        static void CloseOperation(IAsyncInfo operation){if(operation==null)return;try{if(operation.Status==AsyncStatus.Started)operation.Cancel();}catch{}try{operation.Close();}catch{}}
        void Tick(object sender,EventArgs args)
        {
            try
            {
                if(DateTimeOffset.UtcNow>=deadline){Close();return;}
                if(!requested)
                {
                    // The API's foreground/UI-thread requirement is checked
                    // before requesting consent. A background process never
                    // changes permissions or pretends that access was granted.
                    if(GetForegroundWindow()!=Handle)return;
                    requested=true;access=Geolocator.RequestAccessAsync();status.Text="请在 Windows 提示中决定是否允许定位。\n取消或拒绝只会返回未确认，不会自动修改权限。";return;
                }
                if(position==null)
                {
                    if(access.Status==AsyncStatus.Started)return;
                    if(access.Status!=AsyncStatus.Completed||access.GetResults()!=GeolocationAccessStatus.Allowed){Close();return;}
                    Geolocator locator=new Geolocator();locator.DesiredAccuracy=PositionAccuracy.High;locator.DesiredAccuracyInMeters=10;
                    readAfter=DateTimeOffset.UtcNow;position=locator.GetGeopositionAsync(TimeSpan.Zero,TimeSpan.FromSeconds(timeout));deadline=DateTimeOffset.UtcNow.AddSeconds(timeout+1);status.Text="权限已允许，正在独立读取新鲜系统位置…\n门户配置回显不会作为此次读取结果。";return;
                }
                if(position.Status==AsyncStatus.Started)return;
                if(position.Status!=AsyncStatus.Completed){Close();return;}
                Geocoordinate coordinate=position.GetResults().Coordinate;BasicGeoposition point=coordinate.Point.Position;
                if(!ValidObservation(point.Latitude,point.Longitude,coordinate.Accuracy,(int)coordinate.PositionSource,coordinate.Timestamp,readAfter>after?readAfter:after,DateTimeOffset.UtcNow)){Close();return;}
                Result=new{schemaVersion=1,status="observed",position=new{latitude=point.Latitude,longitude=point.Longitude},timestamp=coordinate.Timestamp.UtcDateTime.ToString("o",CultureInfo.InvariantCulture),accuracyMetres=coordinate.Accuracy,positionSource="default"};Close();
            }
            catch { Close(); }
        }
    }
    [STAThread] static int Main()
    {
        try
        {
            if(!OrdinaryUser())throw new InvalidOperationException();
            // Read a bounded line without accepting a second command.
            System.Text.StringBuilder input=new System.Text.StringBuilder();int next;
            while((next=Console.In.Read())>=0 && next!='\n') { if(input.Length>=1024||next=='\r')throw new InvalidOperationException();input.Append((char)next); }
            DateTimeOffset after;int timeout;if(!ReadRequest(input.ToString(),out after,out timeout))throw new InvalidOperationException();
            Application.SetUnhandledExceptionMode(UnhandledExceptionMode.ThrowException);Application.EnableVisualStyles();Application.SetCompatibleTextRenderingDefault(false);
            using(ReadForm form=new ReadForm(after,timeout)){Application.Run(form);if(form.Result==null)throw new InvalidOperationException();Reply(form.Result);return 0;}
        }
        catch { try { Reply(new{schemaVersion=1,status="unconfirmed"}); }catch{} return 1; }
    }
}
