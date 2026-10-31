package browserenginehost

import (
	"context"
	"encoding/json"
	"reflect"
	"strconv"
	"strings"

	"github.com/Maoyangui/godusevpn/internal/browserhost"
)

// protocolLaunch contains only values produced by the native launcher. None of
// these paths, ports or command-line arguments may be supplied by a web view.
type protocolLaunch struct {
	RelayPort        int
	ProfileDirectory string
	Arguments        []string
}

type protocolDriver interface {
	Configure(context.Context) error
	Navigate(context.Context, string) error
	ExportCookies(context.Context) (json.RawMessage, error)
	ImportCookies(context.Context, json.RawMessage) error
	Close()
}

const workerCSP = "worker-src 'none'; object-src 'none'"

// This fixed source is installed before author scripts, in every Window realm.
// It does not change navigator identity or synthesize permission-query results.
// Network CSP and native worker-target rejection form additional boundaries.
const protocolPreload = `(()=>{'use strict';if(Object.getOwnPropertyDescriptor(globalThis,'__ultraWorkerBoundary')?.value===true)return;const deny=function(){throw new DOMException('Disabled by browser profile','NotAllowedError')};const put=(o,k,v)=>Object.defineProperty(o,k,{value:v,writable:false,configurable:false});for(const k of ['Worker','SharedWorker'])put(globalThis,k,deny);if(globalThis.Worklet)put(Worklet.prototype,'addModule',deny);if(globalThis.ServiceWorkerContainer)put(ServiceWorkerContainer.prototype,'register',deny);put(globalThis,'open',()=>null);put(globalThis,'__ultraWorkerBoundary',true)})()`

const protocolNativeIdentity = `(async()=>{const u=navigator.userAgentData;if(!u)throw Error('native client hints unavailable');const h=await u.getHighEntropyValues(['fullVersionList','platformVersion','architecture','bitness','model']);for(const key of ['fullVersionList','platformVersion','architecture','bitness','model'])if(!(key in h))throw Error('native client hints incomplete');return JSON.stringify({userAgent:navigator.userAgent,navigatorPlatform:navigator.platform,maxTouchPoints:navigator.maxTouchPoints,devicePixelRatio,clientHints:{brands:u.brands,fullVersionList:h.fullVersionList,platform:u.platform,platformVersion:h.platformVersion,architecture:h.architecture,bitness:h.bitness,mobile:u.mobile,model:h.model}})})()`

const protocolNativeFirefoxIdentity = `JSON.stringify({userAgent:navigator.userAgent,navigatorPlatform:navigator.platform,maxTouchPoints:navigator.maxTouchPoints,devicePixelRatio,oscpu:navigator.oscpu})`

// Runs only in the fresh, controlled about:blank document. Each test frame
// reports its own real navigator/Intl values, including an opaque-origin frame.
// A missing preload, wrong locale, failed frame or timeout refuses readiness.
const protocolReadback = `(async()=>{const collect=async()=>{const u=navigator.userAgentData;let ch=null;if(u){const h=await u.getHighEntropyValues(['fullVersionList','platformVersion','architecture','bitness','model']);ch={brands:u.brands,fullVersionList:h.fullVersionList,platform:u.platform,platformVersion:h.platformVersion,architecture:h.architecture,bitness:h.bitness,mobile:u.mobile,model:h.model}}return {userAgent:navigator.userAgent,navigatorPlatform:navigator.platform,oscpu:navigator.oscpu,maxTouchPoints:navigator.maxTouchPoints,devicePixelRatio,hardwareConcurrency:navigator.hardwareConcurrency,clientHints:ch,language:navigator.language,locale:Intl.DateTimeFormat().resolvedOptions().locale,timeZone:Intl.DateTimeFormat().resolvedOptions().timeZone,januaryOffset:new Date(Date.UTC(2026,0,15,12)).getTimezoneOffset(),julyOffset:new Date(Date.UTC(2026,6,15,12)).getTimezoneOffset(),width:innerWidth,height:innerHeight,boundary:globalThis.__ultraWorkerBoundary===true,workerBlocked:(()=>{try{new Worker('data:text/javascript,');return false}catch(e){return e.name==='NotAllowedError'}})()}};const top=await collect();const source='<!doctype html><script>Promise.resolve(('+collect.toString()+')()).then(value=>parent.postMessage({ultraProbe:true,value},"*"))<\/script>';const frames=await Promise.all(['same-origin','opaque-origin'].map(kind=>new Promise((resolve,reject)=>{const f=document.createElement('iframe');let done=false;const finish=(err,v)=>{if(done)return;done=true;clearTimeout(timer);removeEventListener('message',on);f.remove();err?reject(err):resolve(v)};const on=e=>{if(e.source===f.contentWindow&&e.data&&e.data.ultraProbe===true)finish(null,{kind,...e.data.value})};const timer=setTimeout(()=>finish(Error('frame readback timeout')),2000);addEventListener('message',on);if(kind==='same-origin')f.srcdoc=source;else f.src='data:text/html,'+encodeURIComponent(source);(document.body||document.documentElement).appendChild(f)})));return JSON.stringify({top,frames})})()`

type protocolObservation struct {
	Language            string                   `json:"language"`
	Locale              string                   `json:"locale"`
	Timezone            string                   `json:"timeZone"`
	JanuaryOffset       int                      `json:"januaryOffset"`
	JulyOffset          int                      `json:"julyOffset"`
	Width               int                      `json:"width"`
	Height              int                      `json:"height"`
	Boundary            bool                     `json:"boundary"`
	WorkerBlocked       bool                     `json:"workerBlocked"`
	Kind                string                   `json:"kind"`
	UserAgent           string                   `json:"userAgent"`
	NavigatorPlatform   string                   `json:"navigatorPlatform"`
	MaxTouchPoints      int                      `json:"maxTouchPoints"`
	DevicePixelRatio    float64                  `json:"devicePixelRatio"`
	ClientHints         *browserhost.ClientHints `json:"clientHints"`
	OSCPU               string                   `json:"oscpu"`
	HardwareConcurrency *int                     `json:"hardwareConcurrency"`
}

func verifyFirefoxIdentityReadback(raw, ua string, c Config, native protocolObservation) error {
	var value struct {
		Top    protocolObservation   `json:"top"`
		Frames []protocolObservation `json:"frames"`
	}
	if json.Unmarshal([]byte(raw), &value) != nil || len(value.Frames) != 2 {
		return ErrRefused
	}
	platform, touch, dpr, oscpu := native.NavigatorPlatform, native.MaxTouchPoints, native.DevicePixelRatio, native.OSCPU
	preset, err := selectedEnvironment(c)
	if err != nil {
		return err
	}
	if preset != nil {
		platform = preset.NavigatorPlatform
		touch = preset.MaxTouchPoints
		dpr = preset.DeviceScaleFactor
		oscpu = firefoxOSCPU(preset)
	}
	for _, o := range append([]protocolObservation{value.Top}, value.Frames...) {
		if o.UserAgent != ua || o.NavigatorPlatform != platform || o.MaxTouchPoints != touch || o.DevicePixelRatio != dpr || o.OSCPU != oscpu || o.ClientHints != nil {
			return ErrRefused
		}
	}
	return nil
}

func verifyChromeIdentityReadback(raw string, plan ChromeIdentity, native protocolObservation) error {
	var value struct {
		Top    protocolObservation   `json:"top"`
		Frames []protocolObservation `json:"frames"`
	}
	if json.Unmarshal([]byte(raw), &value) != nil || len(value.Frames) != 2 {
		return ErrRefused
	}
	touch, dpr := native.MaxTouchPoints, native.DevicePixelRatio
	if plan.MaxTouchPoints >= 0 {
		touch = plan.MaxTouchPoints
	}
	if plan.DeviceScaleFactor > 0 {
		dpr = plan.DeviceScaleFactor
	}
	for _, o := range append([]protocolObservation{value.Top}, value.Frames...) {
		if o.UserAgent != plan.UserAgent || o.NavigatorPlatform != plan.NavigatorPlatform || o.MaxTouchPoints != touch || o.DevicePixelRatio != dpr {
			return ErrRefused
		}
		// navigator.userAgentData is secure-context only. An opaque data frame
		// cannot expose this API; do not invent a CH object for that realm.
		if o.ClientHints == nil {
			if o.Kind != "opaque-origin" {
				return ErrRefused
			}
		} else if !reflect.DeepEqual(*o.ClientHints, plan.ClientHints) {
			return ErrRefused
		}
	}
	return nil
}

func verifyProtocolReadback(raw string, c Config) error {
	if len(raw) == 0 || len(raw) > 16384 || uniqueProtocolJSON([]byte(raw)) != nil {
		return ErrRefused
	}
	var value struct {
		Top    protocolObservation   `json:"top"`
		Frames []protocolObservation `json:"frames"`
	}
	if json.Unmarshal([]byte(raw), &value) != nil || len(value.Frames) != 2 || value.Top.Width != c.Width || value.Top.Height != c.Height {
		return ErrRefused
	}
	seen := map[string]bool{}
	offsets, ok := protocolTimezoneOffsets(c.Timezone)
	if !ok {
		return ErrRefused
	}
	for i, observation := range append([]protocolObservation{value.Top}, value.Frames...) {
		if c.Hardware != nil && (observation.HardwareConcurrency == nil || *observation.HardwareConcurrency != c.Hardware.LogicalProcessors) {
			return ErrRefused
		}
		if observation.Language != c.Language || !strings.EqualFold(observation.Locale, c.Language) || observation.Timezone != c.Timezone || !observation.Boundary || !observation.WorkerBlocked {
			return ErrRefused
		}
		if observation.JanuaryOffset != offsets[0] || observation.JulyOffset != offsets[1] {
			return ErrRefused
		}
		if i > 0 {
			if seen[observation.Kind] || observation.Kind != "same-origin" && observation.Kind != "opaque-origin" {
				return ErrRefused
			}
			seen[observation.Kind] = true
		}
	}
	return nil
}

func protocolTimezoneOffsets(zone string) ([2]int, bool) {
	// Fixed historical dates keep the check deterministic and include DST.
	values := map[string][2]int{"America/Los_Angeles": {480, 420}, "America/New_York": {300, 240}, "Europe/London": {0, -60}, "Europe/Berlin": {-60, -120}, "Asia/Tokyo": {-540, -540}, "Asia/Shanghai": {-480, -480}}
	v, ok := values[zone]
	return v, ok
}

func validateProtocolLaunch(c Config, launch protocolLaunch) error {
	if validateConfig(c) != nil || launch.RelayPort < 1 || launch.RelayPort > 65535 || launch.ProfileDirectory == "" || strings.ContainsAny(launch.ProfileDirectory, "\x00\r\n") || len(launch.Arguments) == 0 {
		return ErrRefused
	}
	return nil
}

func ownedProxyURL(port int) string { return "socks5://127.0.0.1:" + strconv.Itoa(port) }

func chromeEvaluation(raw json.RawMessage) (string, error) {
	var value struct {
		Result struct {
			Type  string `json:"type"`
			Value string `json:"value"`
		} `json:"result"`
		Exception json.RawMessage `json:"exceptionDetails"`
	}
	if json.Unmarshal(raw, &value) != nil || value.Result.Type != "string" || len(value.Exception) != 0 {
		return "", ErrRefused
	}
	return value.Result.Value, nil
}

func firefoxEvaluation(raw json.RawMessage) (string, error) {
	var value struct {
		Type   string `json:"type"`
		Result struct {
			Type  string `json:"type"`
			Value string `json:"value"`
		} `json:"result"`
	}
	if json.Unmarshal(raw, &value) != nil || value.Type != "success" || value.Result.Type != "string" {
		return "", ErrRefused
	}
	return value.Result.Value, nil
}
