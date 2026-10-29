using System;
internal static class DiagnosticPureChecks
{
    static int checks;
    static void Check(bool value) { ++checks; if (!value) throw new Exception("pure_buffer_plan_failed"); }
    public static void Main() {
        // Calls only numeric planning helpers. No token/process/filesystem/WFP
        // import, Run, Info, diagnostic Main or report writer is invoked.
        Check(BrowserReadOnlyDiagnostic.FixedInfoSize(20) == 4);
        Check(BrowserReadOnlyDiagnostic.FixedInfoSize(10) == 56);
        foreach (int kind in new int[] { 1,3,25 }) {
            uint minimum = kind == 3 ? 4u : 16u;
            Check(BrowserReadOnlyDiagnostic.FixedInfoSize(kind) == 0);
            Check(BrowserReadOnlyDiagnostic.ValidInfoProbe(kind,24,minimum));
            Check(BrowserReadOnlyDiagnostic.ValidInfoProbe(kind,122,minimum));
            Check(!BrowserReadOnlyDiagnostic.ValidInfoProbe(kind,5,minimum));
            Check(!BrowserReadOnlyDiagnostic.ValidInfoProbe(kind,122,0));
            Check(!BrowserReadOnlyDiagnostic.ValidInfoProbe(kind,24,minimum-1));
            Check(!BrowserReadOnlyDiagnostic.ValidInfoProbe(kind,122,(1u<<20)+1));
            Check(BrowserReadOnlyDiagnostic.ValidInfoReturn(kind,128,minimum));
            Check(!BrowserReadOnlyDiagnostic.ValidInfoReturn(kind,128,129));
            Check(!BrowserReadOnlyDiagnostic.ValidInfoReturn(kind,128,0));
        }
        foreach (int kind in new int[] {10,20}) {
            uint size=BrowserReadOnlyDiagnostic.FixedInfoSize(kind);
            Check(!BrowserReadOnlyDiagnostic.ValidInfoProbe(kind,24,size));
            Check(BrowserReadOnlyDiagnostic.ValidInfoReturn(kind,size,size));
            Check(!BrowserReadOnlyDiagnostic.ValidInfoReturn(kind,size,size-1));
            Check(!BrowserReadOnlyDiagnostic.ValidInfoReturn(kind,size,size+1));
        }
        Check(!BrowserReadOnlyDiagnostic.ValidInfoProbe(999,122,64));
        Check(!BrowserReadOnlyDiagnostic.ValidInfoReturn(999,64,64));
        Console.WriteLine("pure_numeric_buffer_checks="+checks);
    }
}
