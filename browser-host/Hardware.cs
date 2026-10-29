using System;
using System.Collections.Generic;
using System.Threading.Tasks;

namespace GodUseVPN.Ultra.BrowserHost
{
    // This class never patches a navigator property or changes native hardware.
    // Configuration is sent only through the engine's official CDP command;
    // the JavaScript below is an independent, read-only observation.
    internal sealed class HardwareOptions
    {
        internal int LogicalProcessors;
        internal bool Custom { get { return LogicalProcessors != 0; } }
        internal const string Method = "Emulation.setHardwareConcurrencyOverride";
        internal static HardwareOptions Parse(IDictionary<string, object> profile)
        {
            var result = new HardwareOptions();
            if (!profile.ContainsKey("hardware")) return result;
            var value = LaunchOptions.Object(LaunchOptions.Value(profile, "hardware"));
            LaunchOptions.Keys(value, "mode", "logicalProcessors");
            if (value.Count != 2 || LaunchOptions.Text(value, "mode") != "logical-processors") throw LaunchOptions.Reject("unsupported_hardware_mode");
            result.LogicalProcessors = LaunchOptions.Number(value, "logicalProcessors");
            if (result.LogicalProcessors < 1 || result.LogicalProcessors > 64) throw LaunchOptions.Reject("logical_processors_integer_1_to_64_required");
            return result;
        }
        internal async Task ConfigureTarget(Func<string, object, Task<IDictionary<string, object>>> cdp)
        {
            if (!Custom) return;
            CheckResponse(await cdp(Method, new { hardwareConcurrency = LogicalProcessors }));
            var response = await cdp("Runtime.evaluate", new { expression = "({hardwareConcurrency:navigator.hardwareConcurrency})", returnByValue = true });
            var value = EvaluationValue(response);
            if (LaunchOptions.Number(value, "hardwareConcurrency") != LogicalProcessors) throw LaunchOptions.Reject("hardware_concurrency_readback_failed");
        }
        internal void ValidateFrames(IDictionary<string, object> value)
        {
            if (!Custom) throw LaunchOptions.Reject("no_hardware_override_requested");
            LaunchOptions.Keys(value, "scopes");
            var scopes = LaunchOptions.Value(value, "scopes") as object[];
            string[] expected = { "main", "cpu-same-origin", "cpu-srcdoc", "cpu-aboutblank" };
            if (scopes == null || scopes.Length != expected.Length) throw LaunchOptions.Reject("hardware_frame_coverage_missing");
            for (int index = 0; index < expected.Length; index++)
            {
                var scope = LaunchOptions.Object(scopes[index]); LaunchOptions.Keys(scope, "name", "logicalProcessors");
                if (LaunchOptions.Text(scope, "name") != expected[index] || LaunchOptions.Number(scope, "logicalProcessors") != LogicalProcessors) throw LaunchOptions.Reject("hardware_frame_readback_failed");
            }
        }
        private static void CheckResponse(IDictionary<string, object> value)
        { if (value == null || value.ContainsKey("error")) throw LaunchOptions.Reject("hardware_protocol_refused"); }
        internal static IDictionary<string, object> EvaluationValue(IDictionary<string, object> response)
        {
            CheckResponse(response);
            if (response.ContainsKey("exceptionDetails")) throw LaunchOptions.Reject("hardware_evaluation_failed");
            return LaunchOptions.Object(LaunchOptions.Value(LaunchOptions.Object(LaunchOptions.Value(response, "result")), "value"));
        }
        internal const string ProbeFrames = @"(()=>{
'use strict';
const scopes=[{name:'main',logicalProcessors:navigator.hardwareConcurrency}];
for(const name of ['cpu-same-origin','cpu-srcdoc','cpu-aboutblank']){
 const frame=document.getElementById(name);
 if(!frame||!frame.contentWindow||!frame.contentDocument||frame.contentDocument.readyState!=='complete')throw Error('hardware_owned_frame_not_ready');
 scopes.push({name,logicalProcessors:frame.contentWindow.navigator.hardwareConcurrency});
}
return {scopes};
})()";
    }
}
