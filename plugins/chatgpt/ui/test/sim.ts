// A simulated host for the widget. It plays the three parts that the widget talks to: the MCP Apps host that carries
// its tool calls and opens its links, the OpenAI extensions that carry the model context and the composer, and the page
// with its document and its window. The widget runs here exactly as it runs in the conversation, so a test can open it
// with a tool result, click what a person clicks, and read what a person reads (011:106).
import type {Extensions,Host,HostContext,Page,ToolResult} from "../src/widget.ts";

// FakeElement is the small part of a DOM element that the widget uses.
export class FakeElement {
 tagName:string;
 textContent="";
 className="";
 disabled=false;
 value="";
 children:FakeElement[]=[];
 attributes=new Map<string,string>();
 listeners=new Map<string,Array<(event:{preventDefault():void})=>void>>();
 [property:string]:unknown;
 constructor(tag:string){this.tagName=tag;}
 append(...nodes:FakeElement[]):void{this.children.push(...nodes);}
 replaceChildren(...nodes:FakeElement[]):void{this.children=nodes;}
 setAttribute(name:string,value:string):void{this.attributes.set(name,value);}
 addEventListener(type:string,listener:(event:{preventDefault():void})=>void):void{
  const list=this.listeners.get(type)??[];list.push(listener);this.listeners.set(type,list);
 }
 // text is what a person reads: this element's own text, then its children in order.
 text():string{
  const parts:string[]=[];if(this.textContent)parts.push(this.textContent);
  for(const child of this.children)parts.push(child.text());
  return parts.join("\n");
 }
 all():FakeElement[]{return [this,...this.children.flatMap(child=>child.all())];}
 buttons(label:string):FakeElement[]{return this.all().filter(node=>node.tagName==="button"&&node.textContent===label);}
}

// settle lets every promise that the widget started finish.
export async function settle():Promise<void>{for(let turn=0;turn<6;turn++)await new Promise<void>(resolve=>setTimeout(resolve,0));}

export class FakeDocument {
 app=new FakeElement("div");
 querySelector(_selector:string):FakeElement{return this.app;}
 createElement(tag:string):FakeElement{return new FakeElement(tag);}
}

export type Handler=(args:Record<string,unknown>)=>ToolResult|Promise<ToolResult>;

export class SimHost implements Host {
 calls:Array<{name:string;args:Record<string,unknown>}>=[];
 opened:string[]=[];
 displayModes:string[]=[];
 server:Record<string,Handler>={};
 context:HostContext|undefined={availableDisplayModes:["inline","fullscreen"],displayMode:"inline"};
 // A host grants a request for another display mode, and announces the change in the host context. The MCP Apps examples
 // only answer with the mode that was set, so a test can turn the announcement off.
 grants=true;
 announces=true;
 ontoolresult:((result:ToolResult)=>void)|undefined;
 onhostcontextchanged:((changed?:HostContext)=>void)|undefined;
 async callServerTool(params:{name:string;arguments:Record<string,unknown>}):Promise<ToolResult>{
  this.calls.push({name:params.name,args:params.arguments});
  const handler=this.server[params.name];
  if(!handler)return {isError:true,structuredContent:{reason:"the server has no such tool"}};
  return handler(params.arguments);
 }
 async openLink(params:{url:string}):Promise<void>{this.opened.push(params.url);}
 getHostContext():HostContext|undefined{return this.context;}
 async requestDisplayMode(params:{mode:"inline"|"fullscreen"}):Promise<{mode?:string}|undefined>{
  this.displayModes.push(params.mode);
  const mode=this.grants?params.mode:(this.context?.displayMode??"inline");
  if(this.grants&&this.announces)this.setMode(mode);
  return {mode};
 }
 // setMode is the host changing the display mode and saying so, as when the person closes the fullscreen view.
 setMode(mode:string):void{this.context={...this.context,displayMode:mode};this.onhostcontextchanged?.({displayMode:mode});}
 async connect():Promise<void>{}
 // deliver is the host passing the widget the result of the tool call that opened it.
 deliver(structuredContent:unknown):void{this.ontoolresult?.({structuredContent});}
 // names lists the server tools that were called, in order.
 names():string[]{return this.calls.map(call=>call.name);}
}

export class SimExtensions implements Extensions {
 updates:Array<{content:unknown;structuredContent:unknown}>=[];
 sent:Array<{role:string;content:unknown}>=[];
 current:{updateId?:string}|undefined;
 link:{url:string}|undefined;
 counter=0;
 modelContext={
  update:async(params:{content:unknown;structuredContent:unknown})=>{this.updates.push(params);this.counter++;this.current={updateId:`update-${this.counter}`};return this.current;},
  getCurrent:()=>this.current,
 };
 message={send:async(params:{role:"user";content:unknown})=>{this.sent.push(params);}};
 deepLink={getCurrent:()=>this.link};
}

export class SimPage {
 document=new FakeDocument();
 listeners=new Map<string,Array<()=>void>>();
 // styled holds each host context that the widget passed on for the page to take its theme and variables from.
 styled:Array<HostContext|undefined>=[];
 window={
  openai:undefined as {openExternal?:(args:{href:string})=>void}|undefined,
  addEventListener:(type:string,listener:()=>void)=>{const list=this.listeners.get(type)??[];list.push(listener);this.listeners.set(type,list);},
 };
 asPage():Page{return {document:this.document as unknown as Document,window:this.window,styleHost:context=>{this.styled.push(context);}};}
 fire(type:string):void{for(const listener of this.listeners.get(type)??[])listener();}
 get root():FakeElement{return this.document.app;}
}

// click does what a person does to a button: the widget's own listener runs, and the promises that it started finish.
// A disabled button takes no click, as in a browser.
export async function click(button:FakeElement|undefined):Promise<void>{
 if(!button)throw new Error("the page has no such button");
 if(!button.disabled)for(const listener of button.listeners.get("click")??[])listener({preventDefault(){}});
 await settle();
}
