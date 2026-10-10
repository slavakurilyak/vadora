import {AppBridge,PostMessageTransport} from "@modelcontextprotocol/ext-apps/app-bridge";
import {World,type Json,type OrderState,type Scenario} from "./world.ts";

// This page is a small MCP Apps host. It embeds the real widget in an iframe, talks to it through the real AppBridge, and
// answers its tool calls from the fake server in world.ts, so a person can see and click every view of the widget in a
// browser on their own machine, in the inline card and in fullscreen, in light and dark, and at larger text sizes. The page
// is a preview. ChatGPT draws the widget with its own chrome, its own style variables, and its own limits.

type Mode="inline"|"fullscreen";
type Theme="light"|"dark";

const query=new URLSearchParams(location.search);
const field=(id:string):HTMLSelectElement=>document.getElementById(id) as HTMLSelectElement;
const stage=document.getElementById("stage") as HTMLElement;
const holder=document.getElementById("frame") as HTMLElement;
const logBox=document.getElementById("log") as HTMLElement;

for(const id of ["scenario","mode","theme","scale","width","order","variables"]) {
 const wanted=query.get(id);
 if(wanted!==null)field(id).value=wanted;
 field(id).addEventListener("change",()=>{void start();});
}
if(query.get("controls")==="0")document.body.classList.add("bare");

// hostVariables stands in for the style variables that a host passes. The names are the MCP Apps style variable names.
const hostVariables:Record<Theme,Record<string,string>>={
 light:{"--color-background-primary":"#ffffff","--color-text-primary":"#0d0d0d","--color-text-secondary":"#5d5d5d","--color-border-primary":"#0d0d0d26"},
 dark:{"--color-background-primary":"#212121","--color-text-primary":"#f3f3f3","--color-text-secondary":"#b4b4b4","--color-border-primary":"#ffffff33"},
};

function log(line:string):void {
 logBox.textContent=`${new Date().toISOString().slice(11,19)}  ${line}\n${logBox.textContent??""}`.slice(0,6000);
}

// layout sizes the stage and the frame for a display mode. Inline, the frame is a card of the chosen width whose height
// follows the widget. Fullscreen, the frame fills the window.
function layout(frame:HTMLIFrameElement,mode:Mode):void {
 stage.className=`${mode} ${field("theme").value}`;
 stage.style.setProperty("--width",`${field("width").value}px`);
 frame.style.height=mode==="fullscreen"?"calc(100vh - 8rem)":"120px";
 document.getElementById("close")!.hidden=mode!=="fullscreen";
}

let session=0;
let current:AppBridge|undefined;
async function start():Promise<void> {
 const mine=++session;
 try {await current?.close();} catch {/* the previous bridge is already closed */}
 const scenario=field("scenario").value as Scenario;
 const order=field("order").value as OrderState;
 let mode=field("mode").value as Mode;
 const theme=field("theme").value as Theme;
 const world=new World(scenario,order);
 holder.querySelector("iframe")?.remove();
 const frame=document.createElement("iframe");
 frame.title="Vadora widget";
 frame.setAttribute("sandbox","allow-scripts allow-same-origin allow-popups");
 holder.append(frame);
 layout(frame,mode);
 const context:Record<string,unknown>={displayMode:mode,availableDisplayModes:["inline","fullscreen"],theme};
 if(field("variables").value==="host")context.styles={variables:hostVariables[theme]};
 const bridge=new AppBridge(null,{name:"Vadora preview host",version:"1.0.0"},{
  serverTools:{},openLinks:{},updateModelContext:{text:{}},message:{text:{}},
  experimental:{"openai/message":{},"openai/modelContext":{}},
 },{hostContext:context as never});
 current=bridge;
 let counter=0;
 bridge.oncalltool=async params=>{
  const args=(params.arguments??{}) as Json;
  const reply=world.call(params.name,args);
  log(`tool call ${params.name} ${JSON.stringify(args)}${reply.isError?" (error)":""}`);
  return {content:[],...reply};
 };
 bridge.onopenlink=async({url})=>{log(`open link ${url}`);return {};};
 bridge.onmessage=async params=>{log(`message to the composer ${JSON.stringify(params.content)}`);return {};};
 bridge.onupdatemodelcontext=async params=>{
  counter++;
  const updateId=`update-${counter}`;
  log(`model context ${JSON.stringify(params.structuredContent)}`);
  setTimeout(()=>{void bridge.sendHostContextChange({"openai/modelContext":{updateId}} as never);},0);
  return {_meta:{"openai/modelContext":{updateId}}} as never;
 };
 bridge.onsizechange=({height})=>{
  if(mine===session&&mode==="inline"&&height!==undefined)frame.style.height=`${Math.ceil(height)}px`;
 };
 bridge.onrequestdisplaymode=async({mode:requested})=>{
  log(`display mode requested ${requested}`);
  mode=requested==="fullscreen"?"fullscreen":"inline";
  layout(frame,mode);
  await bridge.sendHostContextChange({displayMode:mode} as never);
  return {mode} as never;
 };
 document.getElementById("close")!.onclick=()=>{
  mode="inline";
  layout(frame,mode);
  void bridge.sendHostContextChange({displayMode:"inline"} as never);
 };
 const initialized=new Promise<void>(resolve=>{bridge.oninitialized=()=>resolve();});
 frame.src="/frame.html";
 await bridge.connect(new PostMessageTransport(frame.contentWindow!,frame.contentWindow!));
 await initialized;
 if(mine!==session)return;
 frame.contentDocument!.documentElement.style.fontSize=`${field("scale").value}%`;
 await bridge.sendToolResult({content:[],structuredContent:world.initial(scenario)} as never);
 log(`opened ${scenario} in ${mode}`);
}

void start();
