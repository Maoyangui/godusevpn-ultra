using System;
using System.Web.Script.Serialization;
internal static class LocationConsumerPureTests
{
 static int checks;
 static void Check(bool condition) { checks++;if(!condition)throw new Exception("pure check failed"); }
 static int Main()
 {
  DateTimeOffset now=DateTimeOffset.UtcNow;DateTimeOffset after;int timeout;
  string stamp=now.UtcDateTime.ToString("o");
  Check(LocationConsumer.ReadRequest("{\"schemaVersion\":1,\"requestedAfter\":\""+stamp+"\",\"timeoutSeconds\":12}",out after,out timeout));
  foreach(string raw in new[]{"{}","{\"schemaVersion\":2}","{\"schemaVersion\":1,\"requestedAfter\":\""+stamp+"\",\"timeoutSeconds\":1}","{\"schemaVersion\":1,\"requestedAfter\":\""+stamp+"\",\"timeoutSeconds\":12,\"extra\":true}"})Check(!LocationConsumer.ReadRequest(raw,out after,out timeout));
  string nanos=now.UtcDateTime.ToString("yyyy-MM-ddTHH:mm:ss")+".123456789Z";
  // .NET's parser can round nanoseconds. No consumer acceptance depends on a
  // string equality; both sides validate the actual fresh UTC timestamp.
  DateTimeOffset parsed;Check(DateTimeOffset.TryParse(nanos,out parsed));
  Check(LocationConsumer.ValidObservation(1,2,10,5,now,now,now));
  Check(!LocationConsumer.ValidObservation(1,2,10,0,now,now,now));
  Check(!LocationConsumer.ValidObservation(1,2,10,5,now.AddSeconds(-1),now,now));
  Check(!LocationConsumer.ValidObservation(1,2,10,5,now.AddSeconds(3),now,now));
  Check(!LocationConsumer.ValidObservation(double.NaN,2,10,5,now,now,now));
  Check(!LocationConsumer.ValidObservation(91,2,10,5,now,now,now));
  Check(!LocationConsumer.ValidObservation(1,2,0,5,now,now,now));
  string serialized=new JavaScriptSerializer().Serialize(new{schemaVersion=1,status="observed",position=new{latitude=1.0,longitude=2.0},timestamp=stamp,accuracyMetres=10.0,positionSource="default"});Check(serialized.Contains("\"positionSource\":\"default\""));
  Console.WriteLine("LocationConsumer pure checks: "+checks);return 0;
 }
}
